package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

// cmdWebdata writes what the website reads: data/leaderboard.json and the test tasks for
// the browser runner (data/tasks/decide.json, data/tasks/gen.json).
func cmdWebdata(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: lvwb webdata <web/data dir> [results dir]")
	}
	out := args[0]
	if err := os.MkdirAll(filepath.Join(out, "tasks"), 0o755); err != nil {
		return err
	}
	type decideRow struct {
		ID        string                     `json:"id"`
		Workflow  string                     `json:"workflow"`
		State     json.RawMessage            `json:"state"`
		Questions map[string]schema.Question `json:"questions"`
		Gold      map[string]schema.Answer   `json:"gold"`
	}
	var dec []decideRow
	for _, wf := range tasks.Names("decide") {
		items, _ := tasks.DecideItems(wf, "test")
		for _, it := range items {
			dec = append(dec, decideRow{it.ID, it.Workflow, it.State, it.Questions, it.Gold})
		}
	}
	var gen []schema.GenItem
	for _, c := range tasks.Names("gen") {
		items, err := tasks.GenItems(c, 0)
		if err != nil {
			return err
		}
		gen = append(gen, items...)
	}
	write := func(name string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(out, name), b, 0o644)
	}
	if err := write("tasks/decide.json", dec); err != nil {
		return err
	}
	if err := write("tasks/gen.json", gen); err != nil {
		return err
	}
	dir := runnerResults(args)
	rs, _ := results.LoadDir(dir)
	return write("leaderboard.json", buildBoard(rs, "standard"))
}

func runnerResults(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return "results"
}
