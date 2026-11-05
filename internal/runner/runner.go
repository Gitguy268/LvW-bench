// Package runner runs the LvW-bench tracks against one model and scores the outputs.
package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Gitguy268/LvW-bench/internal/backend"
	"github.com/Gitguy268/LvW-bench/internal/hub"
	"github.com/Gitguy268/LvW-bench/internal/resolve"
	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

// Options of one run.
type Options struct {
	Model           string
	Resolve         resolve.Options
	Backend         string // auto, llama, hf, openai, ollama
	ServerModel     string // model name for openai/ollama servers
	APIKey          string
	Tracks          []string
	Configs         []string
	Profile         string
	Limit           int
	Seed            int64
	Parallel        int
	ReasonBudget    int
	CPUOnly         bool
	TrustRemoteCode bool
	DType           string
	Base            string
	Adapter         string // auto, systemone, logprobs, sampled
	Mode            string // general or specialist
	Notes           string
	Events          func(Event)
}

// Event reports progress to the CLI, TUI or app.
type Event struct {
	Kind    string `json:"kind"` // status, download, progress, track_done, done
	Msg     string `json:"msg,omitempty"`
	Track   string `json:"track,omitempty"`
	Config  string `json:"config,omitempty"`
	Done    int    `json:"done,omitempty"`
	Total   int    `json:"total,omitempty"`
	Correct int    `json:"correct,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
	OfBytes int64  `json:"of_bytes,omitempty"`
}

func (o *Options) emit(e Event) {
	if o.Events != nil {
		o.Events(e)
	}
}

// Run resolves the model, starts its server, runs the tracks and returns the result.
func Run(ctx context.Context, o Options) (*results.Result, []results.Prediction, error) {
	if o.Profile == "" {
		o.Profile = "standard"
	}
	prof, ok := tasks.Profiles[o.Profile]
	if !ok {
		return nil, nil, fmt.Errorf("unknown profile %q (quick, standard, full)", o.Profile)
	}
	if o.Parallel <= 0 {
		o.Parallel = 4
	}
	if o.ReasonBudget <= 0 {
		o.ReasonBudget = 2048
	}
	if o.Mode == "" {
		o.Mode = "general"
	}
	if len(o.Tracks) == 0 {
		o.Tracks = schema.AllTracks
	}
	srv, model, err := Start(ctx, &o)
	if err != nil {
		return nil, nil, err
	}
	defer srv.Close()

	mods := srv.Client.Modalities(ctx)
	if len(mods) == 0 {
		mods = []string{"text"}
	}
	res := &results.Result{
		Bench: "LvW-bench", BenchVersion: results.Version, Created: time.Now().UTC(), Model: model,
		Mode: o.Mode, Profile: o.Profile, Seed: o.Seed, Tracks: map[string]*results.Track{}, Notes: o.Notes,
		System: map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": fmt.Sprint(runtime.NumCPU()), "lvwb": results.Version},
	}
	if o.CPUOnly {
		res.System["device"] = "cpu"
	}
	if srv.Version != "" {
		res.System["engine"] = srv.Version
	}
	var preds []results.Prediction
	for _, tr := range o.Tracks {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		var t *results.Track
		var p []results.Prediction
		switch tr {
		case schema.TrackDecide:
			t, p, err = runDecide(ctx, &o, srv.Client, mods, prof)
		case schema.TrackAnswer, schema.TrackReason:
			if !backend.Has(mods, "text") {
				o.emit(Event{Kind: "status", Track: tr, Msg: "skipped: the model is a decision model without text generation"})
				continue
			}
			t, p, err = runGen(ctx, &o, srv.Client, tr, prof)
		default:
			return nil, nil, fmt.Errorf("unknown track %q (decide, answer, reason)", tr)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s track: %w", tr, err)
		}
		if t != nil {
			res.Tracks[tr] = t
			preds = append(preds, p...)
			o.emit(Event{Kind: "track_done", Track: tr, Msg: fmt.Sprintf("accuracy %.3f", t.Accuracy)})
		}
	}
	if len(res.Tracks) == 0 {
		return nil, nil, errors.New("no track could run on this model")
	}
	o.emit(Event{Kind: "done"})
	return res, preds, nil
}

// Start resolves the model and starts (or connects to) its server.
func Start(ctx context.Context, o *Options) (*backend.Server, results.Model, error) {
	var m results.Model
	m.Input = o.Model
	switch o.Backend {
	case "ollama":
		base := os.Getenv("OLLAMA_HOST")
		if base == "" {
			base = "http://127.0.0.1:11434"
		} else if !strings.HasPrefix(base, "http") {
			base = "http://" + base
		}
		m.Label, m.Format, m.Backend = o.Model+" · ollama", "ollama", "ollama"
		s := backend.External(base, o.Model, "ollama")
		return s, m, nil
	case "openai":
		base := o.Model
		name := o.ServerModel
		m.Label, m.Format, m.Backend = strings.TrimSpace(name+" · "+hostOf(base)), "server", "openai"
		s := backend.External(base, name, "openai")
		s.Client.APIKey = o.APIKey
		return s, m, nil
	}
	o.emit(Event{Kind: "status", Msg: "resolving " + o.Model})
	spec, err := resolve.Resolve(ctx, o.Model, o.Resolve)
	if err != nil {
		return nil, m, err
	}
	m.Label, m.Format, m.Repo, m.SHA, m.File, m.Quant, m.SizeBytes = spec.Label(), spec.Kind, spec.Repo, spec.SHA, spec.File, spec.Quant, spec.SizeBytes
	if spec.Subfolder != "" {
		m.File = spec.Subfolder + "/"
	}
	switch spec.Kind {
	case resolve.KindServer:
		m.Backend = "openai"
		s := backend.External(spec.BaseURL, o.ServerModel, "openai")
		s.Client.APIKey = o.APIKey
		return s, m, nil
	case resolve.KindGGUF:
		m.Backend = "llama.cpp"
		local := spec.Local
		if spec.HubRepo() != nil {
			if err := checkDisk(spec.SizeBytes); err != nil {
				return nil, m, err
			}
			for i, part := range spec.Parts {
				p, err := hub.Download(ctx, spec.HubRepo(), part, func(f string, done, total int64) {
					o.emit(Event{Kind: "download", Msg: f, Bytes: done, OfBytes: total})
				})
				if err != nil {
					return nil, m, err
				}
				if i == 0 {
					local = p
				}
			}
		}
		o.emit(Event{Kind: "status", Msg: "starting llama-server"})
		perSlot := 2048 + o.ReasonBudget + 768
		s, err := backend.StartLlama(ctx, local, backend.LlamaOptions{Ctx: perSlot, Parallel: o.Parallel, GPU: !o.CPUOnly})
		return s, m, err
	default:
		m.Backend = "transformers"
		o.emit(Event{Kind: "status", Msg: "starting the Python helper (first run installs torch with uv)"})
		h := backend.HelperOptions{Local: spec.Local, Repo: spec.Repo, Revision: spec.SHA, Subfolder: spec.Subfolder,
			TrustRemoteCode: o.TrustRemoteCode, DType: o.DType, Base: o.Base}
		s, err := backend.StartHelper(ctx, h)
		return s, m, err
	}
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.Index(u, "/"); i >= 0 {
		u = u[:i]
	}
	return u
}

func pickN[T any](xs []T, n int) []T { return tasks.Spread(xs, n) }

func configsFor(o *Options, kind string) []string {
	if len(o.Configs) == 0 {
		return tasks.Names(kind)
	}
	var out []string
	for _, c := range o.Configs {
		if cfg, ok := tasks.Find(c); ok && cfg.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// pool runs fn over n jobs with o.Parallel workers.
func pool(ctx context.Context, n, workers int, fn func(i int)) {
	var wg sync.WaitGroup
	jobs := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(i)
			}
		}()
	}
	for i := 0; i < n && ctx.Err() == nil; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func round(x float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(x*p) / p
}

// StateResultsDir is the default results folder.
func StateResultsDir() string { return filepath.Join(backend.StateDir(), "results") }
