package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/score"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

// reference rows for the decide track: Prior (train label frequencies, ignores the input)
// and Uniform (same probability on every option), scored on the whole test split.
func referenceRows() map[string]*results.Track {
	out := map[string]*results.Track{}
	for _, name := range []string{"Prior", "Uniform"} {
		var all []score.DecisionScore
		per := map[string]results.ConfigScore{}
		for _, wf := range tasks.Names("decide") {
			items, _ := tasks.DecideItems(wf, "test")
			prior := tasks.Prior(wf)
			var ss []score.DecisionScore
			for _, it := range items {
				for _, q := range it.QuestionIDs() {
					var pred map[string]float64
					if name == "Prior" {
						pred = prior[q]
					}
					ss = append(ss, score.ScoreDecision(it.Questions[q], pred, it.Gold[q].Probabilities))
				}
			}
			s := score.SummarizeDecisions(ss)
			per[wf] = results.ConfigScore{N: s.N, Accuracy: round4(s.Accuracy), KL: round4(s.KL), Brier: round4(s.Brier), ECE: round4(s.ECE)}
			all = append(all, ss...)
		}
		s := score.SummarizeDecisions(all)
		out[name] = &results.Track{Accuracy: round4(s.Accuracy), KL: round4(s.KL), Brier: round4(s.Brier), ECE: round4(s.ECE), N: s.N, PerConfig: per}
	}
	return out
}

func round4(x float64) float64 { return float64(int64(x*10000+0.5)) / 10000 }

// boardJSON is what the website and the HF Space read.
type boardJSON struct {
	Bench     string                    `json:"bench"`
	Version   string                    `json:"version"`
	Profile   string                    `json:"profile"`
	Tracks    map[string][]boardRow     `json:"tracks"`
	Reference map[string]*results.Track `json:"reference"`
	Configs   []tasks.Config            `json:"configs"`
	Extra     map[string]map[string]any `json:"-"`
}

type boardRow struct {
	Rank   int               `json:"rank"`
	Label  string            `json:"label"`
	Model  results.Model     `json:"model"`
	Mode   string            `json:"mode"`
	Notes  string            `json:"notes,omitempty"`
	Date   string            `json:"date"`
	System map[string]string `json:"system"`
	Track  *results.Track    `json:"score"`
}

func buildBoard(rs []*results.Result, profile string) boardJSON {
	b := boardJSON{Bench: "LvW-bench", Version: results.Version, Profile: profile, Tracks: map[string][]boardRow{},
		Reference: referenceRows(), Configs: tasks.Configs}
	for _, tr := range schema.AllTracks {
		for _, row := range results.Leaderboard(rs, tr, profile) {
			r := row.Result
			b.Tracks[tr] = append(b.Tracks[tr], boardRow{Rank: row.Rank, Label: r.Model.Label, Model: r.Model, Mode: r.Mode,
				Notes: r.Notes, Date: r.Created.Format("2006-01-02"), System: r.System, Track: row.Track})
		}
	}
	return b
}

func printBoardJSON(b boardJSON) {
	out, _ := json.MarshalIndent(b, "", " ")
	fmt.Println(string(out))
}

func hfLink(m results.Model) string {
	if m.Repo == "" {
		return m.Label
	}
	return fmt.Sprintf("[%s](https://huggingface.co/%s)%s", m.Repo, m.Repo, strings.TrimPrefix(m.Label, m.Repo[strings.Index(m.Repo, "/")+1:]))
}

// printBoardMarkdown writes the leaderboard tables of the dataset card and README.
func printBoardMarkdown(b boardJSON) {
	for _, tr := range schema.AllTracks {
		rows := b.Tracks[tr]
		for _, mode := range []string{"general", "specialist"} {
			var sel []boardRow
			for _, r := range rows {
				if r.Mode == mode {
					sel = append(sel, r)
				}
			}
			if len(sel) == 0 && !(tr == schema.TrackDecide && mode == "general") {
				continue
			}
			title := map[string]string{schema.TrackDecide: "System 1 · decide", schema.TrackAnswer: "System 2 · answer", schema.TrackReason: "System 2 · reason"}[tr]
			if mode == "specialist" {
				fmt.Printf("\n#### %s: specialists\n\nTrained on the same task as a workflow (the Spot Snake models learned `spot_snake` moves with the same teacher), so not comparable with the zero-shot table.\n\n", title)
			} else {
				fmt.Printf("\n#### %s\n\n", title)
			}
			if tr == schema.TrackDecide {
				fmt.Println("| # | Model | Accuracy ↑ | KL ↓ | Brier ↓ | ECE ↓ | Coverage | p50 / case | Adapter |")
				fmt.Println("|---|---|---|---|---|---|---|---|---|")
				for i, r := range sel {
					t := r.Track
					fmt.Printf("| %d | %s | %.3f | %.3f | %.3f | %.3f | %.0f%% | %.0f ms | %s |\n", i+1, hfLink(r.Model), t.Accuracy, t.KL, t.Brier, t.ECE, 100*t.Coverage, t.P50Millis, t.Adapter)
				}
				if mode == "general" {
					for _, name := range []string{"Prior", "Uniform"} {
						t := b.Reference[name]
						what := map[string]string{"Prior": "Prior (ignores the input)", "Uniform": "Uniform (same probability on every option)"}[name]
						fmt.Printf("| – | %s | %.3f | %.3f | %.3f | %.3f | – | – | reference |\n", what, t.Accuracy, t.KL, t.Brier, t.ECE)
					}
				}
				continue
			}
			fmt.Println("| # | Model | Accuracy ↑ | 95% CI | gsm8k | mmlu | mmlu_pro | truthfulqa | ifeval | generated | tok/answer | tok/s |")
			fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|")
			for i, r := range sel {
				t := r.Track
				cell := func(c string) string {
					if s, ok := t.PerConfig[c]; ok {
						return fmt.Sprintf("%.3f", s.Accuracy)
					}
					return "–"
				}
				fmt.Printf("| %d | %s | %.3f | %.3f–%.3f | %s | %s | %s | %s | %s | %s | %.0f | %.0f |\n", i+1, hfLink(r.Model), t.Accuracy, t.CILow, t.CIHigh,
					cell("gsm8k"), cell("mmlu"), cell("mmlu_pro"), cell("truthfulqa"), cell("ifeval"), cell("generated"), t.MeanOutTokens, t.TokensPerSecond)
			}
		}
	}
}
