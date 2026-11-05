// Command lvwb runs LvW-bench: a local benchmark for tiny System One (decision) and
// System Two (generative) models, from the terminal.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Gitguy268/LvW-bench/internal/presets"
	"github.com/Gitguy268/LvW-bench/internal/resolve"
	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/runner"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

var version = "dev"

const usage = `lvwb: LvW-bench, a local benchmark for tiny AI models

Usage:
  lvwb run <model> [flags]     benchmark a model (HF link or id, .gguf, safetensors folder, server URL, preset)
  lvwb tui [<model>]           the same with a live terminal UI
  lvwb models                  list presets
  lvwb tasks                   list task sets and tracks
  lvwb results [dir]           list saved results
  lvwb leaderboard [dir]       leaderboard per track from saved results
  lvwb compare <a.json> <b.json>
  lvwb export <result.json> --md|--csv|--eval-results
  lvwb rescore <file.predictions.jsonl>   score saved outputs again (no model needed)
  lvwb submit <result.json>    print how to share a result (never uploads by itself)
  lvwb dataset <out-dir>       write the task sets as JSONL (for the HF dataset build)
  lvwb webdata <dir> [results] write the website's leaderboard and task JSON
  lvwb selftest                offline scorer check
  lvwb version

Tracks: decide (System 1, typed decisions), answer (System 2, direct), reason (System 2, thinking).
Run "lvwb run -h" for run flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "run":
		err = cmdRun(ctx, args, false)
	case "tui":
		err = cmdRun(ctx, args, true)
	case "models":
		cmdModels()
	case "tasks":
		cmdTasks()
	case "results":
		err = cmdResults(args)
	case "leaderboard", "lb":
		err = cmdLeaderboard(args)
	case "compare":
		err = cmdCompare(args)
	case "export":
		err = cmdExport(args)
	case "rescore":
		err = cmdRescore(args)
	case "submit":
		err = cmdSubmit(args)
	case "dataset":
		err = cmdDataset(args)
	case "webdata":
		err = cmdWebdata(args)
	case "selftest":
		err = cmdSelftest()
	case "version", "--version", "-v":
		fmt.Println("lvwb", version, "(bench", results.Version+")")
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "stopped")
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error {
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			*l = append(*l, x)
		}
	}
	return nil
}

// runFlags parses run flags; flags may come before or after the model.
func runFlags(args []string) (*runner.Options, string, bool, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	o := &runner.Options{}
	var tracks, configs listFlag
	track := fs.String("track", "all", "decide, answer, reason or all (comma-separated works)")
	fs.Var(&tracks, "tracks", "alias of --track")
	fs.Var(&configs, "configs", "task sets to run (default: all for each track)")
	fs.StringVar(&o.Backend, "backend", "auto", "auto, llama, hf, openai (an OpenAI-compatible URL), ollama")
	fs.StringVar(&o.ServerModel, "server-model", "", "model name to send to an openai/ollama server")
	fs.StringVar(&o.Profile, "profile", "standard", "quick, standard (leaderboard) or full")
	fs.IntVar(&o.Limit, "limit", 0, "items per task set (overrides the profile)")
	fs.Int64Var(&o.Seed, "seed", 0, "seed for the generated set (0 = the published one)")
	fs.IntVar(&o.Parallel, "parallel", 4, "requests in flight")
	fs.IntVar(&o.ReasonBudget, "reason-budget", 2048, "max output tokens per reason-track answer")
	fs.StringVar(&o.Resolve.Quant, "quant", "", "GGUF quant to pick from a repo, e.g. Q8_0")
	fs.StringVar(&o.Resolve.File, "file", "", "exact file in the repo")
	fs.StringVar(&o.Resolve.Subfolder, "subfolder", "", "weights in a repo subfolder")
	fs.StringVar(&o.Resolve.Revision, "revision", "", "branch, tag, commit or refs/pr/N")
	fs.StringVar(&o.Resolve.Format, "format", "", "force gguf or safetensors when a repo has both")
	fs.BoolVar(&o.CPUOnly, "cpu", false, "do not offload to the GPU")
	fs.BoolVar(&o.TrustRemoteCode, "trust-remote-code", false, "allow custom model code from the repo (safetensors)")
	fs.StringVar(&o.DType, "dtype", "", "safetensors dtype: float32, bfloat16, float16")
	fs.StringVar(&o.Base, "base", "", "base model for an adapter-only repo")
	fs.StringVar(&o.Adapter, "adapter", "auto", "System One adapter: auto, systemone, logprobs, sampled")
	fs.StringVar(&o.Mode, "mode", "general", "general (zero-shot) or specialist (trained on the train split)")
	fs.StringVar(&o.APIKey, "api-key", os.Getenv("LVWB_API_KEY"), "API key for an openai server")
	fs.StringVar(&o.Notes, "notes", "", "free text stored with the result")
	out := fs.String("out", "", "results folder (default: the lvw-bench cache folder)")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	var model string
	var rest []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, "", false, err
		}
		args = fs.Args()
		if len(args) > 0 {
			rest = append(rest, args[0])
			args = args[1:]
		}
	}
	if len(rest) > 0 {
		model = rest[0]
	}
	if len(tracks) > 0 {
		*track = tracks.String()
	}
	if *track != "all" {
		o.Tracks = strings.Split(*track, ",")
	}
	o.Configs = configs
	if p, ok := presets.Find(model); ok {
		model = p.Input
		if p.Mode != "" && o.Mode == "general" {
			o.Mode = p.Mode
			o.Notes = strings.TrimSpace(o.Notes + " trained on Spot Snake moves with the same teacher and text as the spot_snake workflow")
		}
		if *track == "all" {
			o.Tracks = p.Tracks
		}
		if len(p.Args) > 0 {
			extra, _, _, err := runFlags(p.Args)
			if err == nil {
				if o.Resolve.Subfolder == "" {
					o.Resolve.Subfolder = extra.Resolve.Subfolder
				}
				if len(o.Configs) == 0 {
					o.Configs = extra.Configs
				}
			}
		}
	}
	o.Model = model
	if *out == "" {
		*out = runner.StateResultsDir()
	}
	o.Notes = strings.TrimSpace(o.Notes)
	os.Setenv("LVWB_OUT", *out)
	return o, model, *asJSON, nil
}

func cmdRun(ctx context.Context, args []string, tui bool) error {
	o, model, asJSON, err := runFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if tui {
		return runTUI(ctx, o)
	}
	if model == "" {
		return errors.New("give a model: an HF link or id, a .gguf file, a safetensors folder, a server URL or a preset (lvwb models)")
	}
	pr := newPrinter()
	o.Events = pr.event
	res, preds, err := runner.Run(ctx, *o)
	pr.finish()
	if err != nil {
		return err
	}
	path, err := results.Save(os.Getenv("LVWB_OUT"), res, preds)
	if err != nil {
		return err
	}
	if asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	printResult(res)
	fmt.Println("\nsaved", path)
	return nil
}

// printer writes progress lines (one line per track that rewrites itself on a terminal).
type printer struct {
	tty  bool
	last string
	t0   time.Time
}

func newPrinter() *printer {
	st, _ := os.Stderr.Stat()
	return &printer{tty: st != nil && st.Mode()&os.ModeCharDevice != 0, t0: time.Now()}
}

func (p *printer) line(s string) {
	if p.tty {
		fmt.Fprintf(os.Stderr, "\r\033[K%s", s)
	} else if s != p.last {
		fmt.Fprintln(os.Stderr, s)
	}
	p.last = s
}

func (p *printer) event(e runner.Event) {
	switch e.Kind {
	case "status":
		p.newline()
		prefix := ""
		if e.Track != "" {
			prefix = e.Track + ": "
		}
		fmt.Fprintln(os.Stderr, "·", prefix+e.Msg)
	case "download":
		pct := 0.0
		if e.OfBytes > 0 {
			pct = 100 * float64(e.Bytes) / float64(e.OfBytes)
		}
		p.line(fmt.Sprintf("↓ %s %.0f%% (%.2f / %.2f GB)", e.Msg, pct, float64(e.Bytes)/1e9, float64(e.OfBytes)/1e9))
	case "progress":
		if !p.tty && e.Done%25 != 0 && e.Done != e.Total {
			return
		}
		p.line(fmt.Sprintf("%-7s %4d/%-4d  %5.1f%% correct so far  [%s]", e.Track, e.Done, e.Total, 100*float64(e.Correct)/float64(max(1, e.Done)), e.Config))
	case "track_done":
		p.newline()
		fmt.Fprintf(os.Stderr, "✓ %s: %s\n", e.Track, e.Msg)
	}
}

func (p *printer) newline() {
	if p.tty && p.last != "" {
		fmt.Fprintln(os.Stderr)
	}
	p.last = ""
}

func (p *printer) finish() { p.newline() }

func printResult(r *results.Result) {
	fmt.Printf("\n%s  (%s, %s, profile %s)\n", r.Model.Label, r.Model.Backend, r.Mode, r.Profile)
	for _, tr := range schema.AllTracks {
		t := r.Tracks[tr]
		if t == nil {
			continue
		}
		if tr == schema.TrackDecide {
			fmt.Printf("  decide  acc %.3f [%.3f–%.3f]  KL %.3f  Brier %.3f  ECE %.3f  n=%d  adapter=%s  coverage %.0f%%  p50 %.0f ms\n",
				t.Accuracy, t.CILow, t.CIHigh, t.KL, t.Brier, t.ECE, t.N, t.Adapter, 100*t.Coverage, t.P50Millis)
		} else {
			fmt.Printf("  %-7s acc %.3f [%.3f–%.3f]  n=%d  %.0f tok/answer  %.0f tok/s  truncated %.0f%%  %.0fs\n",
				tr, t.Accuracy, t.CILow, t.CIHigh, t.N, t.MeanOutTokens, t.TokensPerSecond, 100*t.TruncationRate, t.WallSeconds)
		}
		for _, c := range sortedConfigs(t.PerConfig) {
			s := t.PerConfig[c]
			if tr == schema.TrackDecide {
				fmt.Printf("      %-12s %.3f  KL %.3f  (n=%d)\n", c, s.Accuracy, s.KL, s.N)
			} else {
				fmt.Printf("      %-12s %.3f  (n=%d)\n", c, s.Accuracy, s.N)
			}
		}
	}
}

func sortedConfigs(m map[string]results.ConfigScore) []string {
	var out []string
	for _, c := range tasks.Configs {
		if _, ok := m[c.Name]; ok {
			out = append(out, c.Name)
		}
	}
	return out
}

func cmdModels() {
	fmt.Println("Presets (lvwb run <name>):")
	for _, p := range presets.All {
		fmt.Printf("  %-24s %-10s %-50s %s\n", p.Name, p.Kind, p.Input, p.Comment)
	}
	fmt.Println("\nAny HF link or id, local .gguf, safetensors folder or OpenAI-compatible URL works too.")
}

func cmdTasks() {
	fmt.Println("Tracks:")
	fmt.Println("  decide  System 1: typed decisions (choice/score/noul), scored on probabilities: accuracy, KL, Brier, ECE")
	fmt.Println("  answer  System 2: direct answers, thinking off, 32-token budget for short answers")
	fmt.Println("  reason  System 2: thinking on (native or step by step), --reason-budget tokens")
	fmt.Println("\nTask sets:")
	for _, c := range tasks.Configs {
		fmt.Printf("  %-12s %-7s %-62s %s, %s\n", c.Name, c.Kind, c.Description, c.Source, c.License)
	}
	fmt.Println("\nProfiles: quick (10 cases / 20 items), standard (50 / 100, reason 40), full (everything)")
}

func resultsDir(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return runner.StateResultsDir()
}

func cmdResults(args []string) error {
	rs, _ := results.LoadDir(resultsDir(args))
	if len(rs) == 0 {
		fmt.Println("no results yet in", resultsDir(args))
		return nil
	}
	for _, r := range rs {
		var parts []string
		for _, tr := range schema.AllTracks {
			if t := r.Tracks[tr]; t != nil {
				parts = append(parts, fmt.Sprintf("%s %.3f", tr, t.Accuracy))
			}
		}
		fmt.Printf("%s  %-48s %-9s %s\n", r.Created.Local().Format("2006-01-02 15:04"), r.Model.Label, r.Profile, strings.Join(parts, "  "))
	}
	return nil
}

func cmdLeaderboard(args []string) error {
	fs := flag.NewFlagSet("leaderboard", flag.ContinueOnError)
	profile := fs.String("profile", "standard", "profile to rank (\"\" = any)")
	md := fs.Bool("md", false, "Markdown tables (dataset card, README)")
	asJSON := fs.Bool("json", false, "JSON for the website and HF Space")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rs, _ := results.LoadDir(resultsDir(fs.Args()))
	if *md || *asJSON {
		b := buildBoard(rs, *profile)
		if *asJSON {
			printBoardJSON(b)
		} else {
			printBoardMarkdown(b)
		}
		return nil
	}
	for _, tr := range schema.AllTracks {
		rows := results.Leaderboard(rs, tr, *profile)
		if len(rows) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", strings.ToUpper(tr))
		for _, row := range rows {
			t := row.Track
			if tr == schema.TrackDecide {
				fmt.Printf("  %2d. %-46s acc %.3f  KL %.3f  Brier %.3f  ECE %.3f  %s\n", row.Rank, row.Result.Model.Label, t.Accuracy, t.KL, t.Brier, t.ECE, row.Result.Mode)
			} else {
				fmt.Printf("  %2d. %-46s acc %.3f [%.3f–%.3f]  %.0f tok/s\n", row.Rank, row.Result.Model.Label, t.Accuracy, t.CILow, t.CIHigh, t.TokensPerSecond)
			}
		}
	}
	return nil
}

func cmdCompare(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: lvwb compare <a.json> <b.json>")
	}
	a, err := results.Load(args[0])
	if err != nil {
		return err
	}
	b, err := results.Load(args[1])
	if err != nil {
		return err
	}
	fmt.Printf("%-14s %-24s %-24s %s\n", "", trunc(a.Model.Label, 24), trunc(b.Model.Label, 24), "diff")
	for _, tr := range schema.AllTracks {
		ta, tb := a.Tracks[tr], b.Tracks[tr]
		if ta == nil || tb == nil {
			continue
		}
		fmt.Printf("%-14s %-24.3f %-24.3f %+.3f\n", tr, ta.Accuracy, tb.Accuracy, tb.Accuracy-ta.Accuracy)
		for _, c := range sortedConfigs(ta.PerConfig) {
			if sb, ok := tb.PerConfig[c]; ok {
				sa := ta.PerConfig[c]
				fmt.Printf("  %-12s %-24.3f %-24.3f %+.3f\n", c, sa.Accuracy, sb.Accuracy, sb.Accuracy-sa.Accuracy)
			}
		}
	}
	return nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func cmdSubmit(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: lvwb submit <result.json>")
	}
	r, err := results.Load(args[0])
	if err != nil {
		return err
	}
	fmt.Printf(`To add %s to the public leaderboard:

  1. Open a discussion on https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/discussions
     titled "Result: %s" and attach:
       %s
       %s
  2. Or open a pull request on https://github.com/Gitguy268/LvW-bench adding both files to results/.
  3. For the Hub leaderboard on the model page, open a PR on the model repo with:
       lvwb export %s --eval-results > .eval_results/lvw-bench.yaml

Nothing was uploaded.
`, r.Model.Label, r.Model.Label, args[0], strings.TrimSuffix(args[0], ".json")+".predictions.jsonl", args[0])
	return nil
}

var _ = resolve.KindGGUF
