// LvW-bench desktop app (Wails v2). It runs the same runner as the lvwb CLI.
package main

import (
	"context"
	"embed"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Gitguy268/LvW-bench/internal/backend"
	"github.com/Gitguy268/LvW-bench/internal/presets"
	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/runner"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "0.1.0"

// App is bound to the frontend as window.go.main.App.
type App struct {
	ctx    context.Context
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx; fixPath() }

// fixPath adds the usual tool folders: apps started from Finder get a minimal PATH.
func fixPath() {
	extra := []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	if h, err := os.UserHomeDir(); err == nil {
		extra = append(extra, h+"/.local/bin", h+"/.cargo/bin")
	}
	sep := string(os.PathListSeparator)
	p := os.Getenv("PATH")
	for _, e := range extra {
		if !strings.Contains(sep+p+sep, sep+e+sep) {
			p += sep + e
		}
	}
	os.Setenv("PATH", p)
}

// Info is shown in the app's header.
type Info struct {
	Version     string           `json:"version"`
	Bench       string           `json:"bench"`
	LlamaServer string           `json:"llama_server"`
	UV          string           `json:"uv"`
	ResultsDir  string           `json:"results_dir"`
	OS          string           `json:"os"`
	Presets     []presets.Preset `json:"presets"`
	Configs     []tasks.Config   `json:"configs"`
}

// Info reports versions, which engines are installed and the presets.
func (a *App) Info() Info {
	ls, _ := backend.FindLlamaServer()
	uv, _ := exec.LookPath("uv")
	return Info{Version: version, Bench: results.Version, LlamaServer: ls, UV: uv, ResultsDir: runner.StateResultsDir(),
		OS: runtime.GOOS + "/" + runtime.GOARCH, Presets: presets.All, Configs: tasks.Configs}
}

// RunRequest is what the form sends.
type RunRequest struct {
	Model           string   `json:"model"`
	Tracks          []string `json:"tracks"`
	Profile         string   `json:"profile"`
	Quant           string   `json:"quant"`
	Subfolder       string   `json:"subfolder"`
	Backend         string   `json:"backend"`
	ServerModel     string   `json:"server_model"`
	Mode            string   `json:"mode"`
	TrustRemoteCode bool     `json:"trust_remote_code"`
	CPUOnly         bool     `json:"cpu_only"`
	Configs         []string `json:"configs"`
}

// RunResponse is the finished run.
type RunResponse struct {
	Result *results.Result `json:"result"`
	Path   string          `json:"path"`
}

// Run benchmarks a model; progress arrives as "lvwb:event" events.
func (a *App) Run(req RunRequest) (*RunResponse, error) {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return nil, errors.New("a run is already going")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.cancel = nil; a.mu.Unlock(); cancel() }()

	o := runner.Options{Model: strings.TrimSpace(req.Model), Tracks: req.Tracks, Profile: req.Profile, Backend: req.Backend,
		ServerModel: req.ServerModel, Mode: req.Mode, TrustRemoteCode: req.TrustRemoteCode, CPUOnly: req.CPUOnly, Configs: req.Configs}
	o.Resolve.Quant, o.Resolve.Subfolder = req.Quant, req.Subfolder
	if o.Backend == "" {
		o.Backend = "auto"
	}
	if p, ok := presets.Find(o.Model); ok {
		o.Model = p.Input
		if len(o.Tracks) == 0 {
			o.Tracks = p.Tracks
		}
		if p.Mode != "" && (o.Mode == "" || o.Mode == "general") {
			o.Mode = p.Mode
		}
		for i := 0; i+1 < len(p.Args); i++ {
			if p.Args[i] == "--subfolder" && o.Resolve.Subfolder == "" {
				o.Resolve.Subfolder = p.Args[i+1]
			}
		}
	}
	o.Events = func(e runner.Event) { wr.EventsEmit(a.ctx, "lvwb:event", e) }
	res, preds, err := runner.Run(ctx, o)
	if err != nil {
		return nil, err
	}
	path, err := results.Save(runner.StateResultsDir(), res, preds)
	if err != nil {
		return nil, err
	}
	return &RunResponse{Result: res, Path: path}, nil
}

// Stop cancels the current run.
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// Results lists saved results, newest first.
func (a *App) Results() []*results.Result {
	rs, _ := results.LoadDir(runner.StateResultsDir())
	return rs
}

// OpenResults shows the results folder in the file manager.
func (a *App) OpenResults() {
	dir := runner.StateResultsDir()
	_ = os.MkdirAll(dir, 0o755)
	wr.BrowserOpenURL(a.ctx, "file://"+dir)
}

func main() {
	app := &App{}
	err := wails.Run(&options.App{
		Title:            "LvW-bench",
		Width:            1180,
		Height:           800,
		MinWidth:         760,
		MinHeight:        560,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 251, G: 250, B: 247, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       func(context.Context) { app.Stop() },
		Bind:             []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
