package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/score"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

// cmdRescore scores a predictions file again with the current scorer, without the model.
func cmdRescore(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: lvwb rescore <result.predictions.jsonl>")
	}
	seed := int64(0)
	if r, err := results.Load(strings.TrimSuffix(args[0], ".predictions.jsonl") + ".json"); err == nil {
		seed = r.Seed
	}
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()
	decide := map[string]map[string]schema.DecideItem{}
	gen := map[string]map[string]schema.GenItem{}
	var ds []score.DecisionScore
	acc := map[string][]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var p results.Prediction
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return err
		}
		if p.Track == schema.TrackDecide {
			if decide[p.Config] == nil {
				items, err := tasks.DecideItems(p.Config, "test")
				if err != nil {
					return err
				}
				decide[p.Config] = map[string]schema.DecideItem{}
				for _, it := range items {
					decide[p.Config][it.ID] = it
				}
			}
			it := decide[p.Config][p.ID]
			ds = append(ds, score.ScoreDecision(it.Questions[p.Question], p.Probabilities, it.Gold[p.Question].Probabilities))
			continue
		}
		if gen[p.Config] == nil {
			items, err := tasks.GenItems(p.Config, seed)
			if err != nil {
				return err
			}
			gen[p.Config] = map[string]schema.GenItem{}
			for _, it := range items {
				gen[p.Config][it.ID] = it
			}
		}
		ok, _ := score.CheckAnswer(gen[p.Config][p.ID], p.Output)
		v := 0.0
		if ok {
			v = 1
		}
		acc[p.Track] = append(acc[p.Track], v)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	out := map[string]any{}
	if len(ds) > 0 {
		out["decide"] = score.SummarizeDecisions(ds)
	}
	var keys []string
	for k := range acc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		xs := acc[k]
		var s float64
		for _, x := range xs {
			s += x
		}
		out[k] = map[string]any{"n": len(xs), "accuracy": s / float64(len(xs))}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return nil
}
