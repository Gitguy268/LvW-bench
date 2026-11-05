package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/score"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

// DatasetID is the HF benchmark dataset.
const DatasetID = "Lucasgitguy-co/LvW-bench"

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	md := fs.Bool("md", false, "Markdown table")
	csvOut := fs.Bool("csv", false, "CSV, one row per track and task set")
	eval := fs.Bool("eval-results", false, "HF .eval_results YAML for a model repo PR")
	var file string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) > 0 {
			file, args = args[0], args[1:]
		}
	}
	if file == "" {
		return errors.New("usage: lvwb export <result.json> --md|--csv|--eval-results")
	}
	r, err := results.Load(file)
	if err != nil {
		return err
	}
	switch {
	case *eval:
		fmt.Print(evalResultsYAML(r))
	case *csvOut:
		w := csv.NewWriter(os.Stdout)
		_ = w.Write([]string{"model", "format", "mode", "profile", "track", "config", "n", "accuracy", "kl_from_gold", "brier", "ece"})
		for _, tr := range schema.AllTracks {
			t := r.Tracks[tr]
			if t == nil {
				continue
			}
			_ = w.Write([]string{r.Model.Label, r.Model.Format, r.Mode, r.Profile, tr, "all", fmt.Sprint(t.N), f4(t.Accuracy), f4(t.KL), f4(t.Brier), f4(t.ECE)})
			for _, c := range sortedConfigs(t.PerConfig) {
				s := t.PerConfig[c]
				_ = w.Write([]string{r.Model.Label, r.Model.Format, r.Mode, r.Profile, tr, c, fmt.Sprint(s.N), f4(s.Accuracy), f4(s.KL), f4(s.Brier), f4(s.ECE)})
			}
		}
		w.Flush()
	default:
		_ = md
		fmt.Printf("| Model | Track | Accuracy | 95%% CI | KL ↓ | Brier ↓ | ECE ↓ | n |\n|---|---|---|---|---|---|---|---|\n")
		for _, tr := range schema.AllTracks {
			if t := r.Tracks[tr]; t != nil {
				kl, br, ece := "–", "–", "–"
				if tr == schema.TrackDecide {
					kl, br, ece = f3(t.KL), f3(t.Brier), f3(t.ECE)
				}
				fmt.Printf("| %s | %s | %.3f | %.3f–%.3f | %s | %s | %s | %d |\n", r.Model.Label, tr, t.Accuracy, t.CILow, t.CIHigh, kl, br, ece, t.N)
			}
		}
	}
	return nil
}

func f3(x float64) string { return fmt.Sprintf("%.3f", x) }
func f4(x float64) string { return fmt.Sprintf("%.4f", x) }

// evalResultsYAML writes the Hub's .eval_results format: one entry per eval.yaml task.
func evalResultsYAML(r *results.Result) string {
	var b strings.Builder
	add := func(task string, v float64) {
		fmt.Fprintf(&b, "- dataset:\n    id: %s\n    task_id: %s\n  value: %.4f\n  date: '%s'\n", DatasetID, task, v, r.Created.Format("2006-01-02"))
		fmt.Fprintf(&b, "  source:\n    url: https://github.com/Gitguy268/LvW-bench\n    name: lvwb %s\n", results.Version)
		fmt.Fprintf(&b, "  notes: '%s, %s, profile %s, %s'\n", r.Mode, strings.ReplaceAll(r.Model.Label, "'", ""), r.Profile, r.Model.Backend)
	}
	if t := r.Tracks[schema.TrackDecide]; t != nil {
		add("decide_accuracy", t.Accuracy)
		add("decide_kl", t.KL)
		add("decide_brier", t.Brier)
		add("decide_ece", t.ECE)
	}
	if t := r.Tracks[schema.TrackAnswer]; t != nil {
		add("answer_accuracy", t.Accuracy)
	}
	if t := r.Tracks[schema.TrackReason]; t != nil {
		add("reason_accuracy", t.Accuracy)
	}
	return b.String()
}

// cmdDataset writes every task set as JSONL: <out>/<config>/<split>.jsonl, the layout the
// HF dataset build (dataset/build_hf.py) turns into parquet.
func cmdDataset(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: lvwb dataset <out-dir>")
	}
	out := args[0]
	write := func(cfg, split string, rows []map[string]any) error {
		dir := filepath.Join(out, cfg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(dir, split+".jsonl"))
		if err != nil {
			return err
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetEscapeHTML(false)
		for _, r := range rows {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		fmt.Printf("%-14s %-5s %d\n", cfg, split, len(rows))
		return nil
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	var allDecide = map[string][]map[string]any{}
	for _, wf := range tasks.Names("decide") {
		for _, split := range []string{"train", "test"} {
			items, _ := tasks.DecideItems(wf, split)
			var rows []map[string]any
			for _, it := range items {
				row := map[string]any{"id": it.ID, "workflow": it.Workflow, "split": split, "state": string(it.State),
					"questions": js(it.Questions), "gold": js(it.Gold), "factors": js(it.Factors), "n_questions": len(it.Questions)}
				for q, g := range it.Gold {
					row[q+"__label"] = g.Label
					row[q+"__confidence"] = g.Confidence
					row[q+"__probabilities"] = js(g.Probabilities)
					if g.Score != nil {
						row[q+"__score"] = *g.Score
					}
					if g.Type == "noul" {
						row[q+"__probability_true"] = g.Probabilities["true"]
					}
				}
				rows = append(rows, row)
				allDecide[split] = append(allDecide[split], map[string]any{"id": it.ID, "workflow": it.Workflow, "split": split,
					"state": row["state"], "questions": row["questions"], "gold": row["gold"], "factors": row["factors"], "n_questions": row["n_questions"]})
			}
			if err := write(wf, split, rows); err != nil {
				return err
			}
		}
	}
	for _, split := range []string{"train", "test"} {
		if err := write("decide_all", split, allDecide[split]); err != nil {
			return err
		}
	}
	var allGen, reasonSubset []map[string]any
	for _, cfg := range tasks.Names("gen") {
		items, err := tasks.GenItems(cfg, 0)
		if err != nil {
			return err
		}
		var rows []map[string]any
		for _, it := range items {
			row := map[string]any{"id": it.ID, "config": it.Config, "prompt": it.Prompt, "choices": it.Choices, "answer": it.Answer,
				"answer_type": it.AnswerType, "meta": js(it.Meta), "source": it.Source, "license": it.License}
			rows = append(rows, row)
			allGen = append(allGen, row)
		}
		reasonSubset = append(reasonSubset, tasks.Spread(rows, tasks.ReasonSubset)...)
		if err := write(cfg, "test", rows); err != nil {
			return err
		}
	}
	if err := write("reason_subset", "test", reasonSubset); err != nil {
		return err
	}
	return write("generative_all", "test", allGen)
}

var _ = score.StripThinking
