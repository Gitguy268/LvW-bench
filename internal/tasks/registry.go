// Package tasks holds the LvW-bench task sets: embedded source subsets, the original System One
// workflows and the seeded System Two generators.
package tasks

import (
	"bufio"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

//go:embed data/*.jsonl.gz
var dataFS embed.FS

// PublishedSeed is the seed of the published `generated` config.
const PublishedSeed = 7

// Config describes one task set.
type Config struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"` // "decide" or "gen"
	Description string `json:"description"`
	Source      string `json:"source"`
	License     string `json:"license"`
}

// Configs lists every task set in display order.
var Configs = []Config{
	{"spot_snake", "decide", "Pick Spot's next move on the lawn without getting boxed in", "original (rule gold)", "MIT"},
	{"it_support", "decide", "Route and prioritise an IT support ticket", "original (rule gold)", "MIT"},
	{"delivery", "decide", "Handle a parcel delivery exception", "original (rule gold)", "MIT"},
	{"code_review", "decide", "Triage a pull request", "original (rule gold)", "MIT"},
	{"calendar", "decide", "Resolve a calendar clash", "original (rule gold)", "MIT"},
	{"gsm8k", "gen", "Grade-school math word problems (200)", "openai/gsm8k test", "MIT"},
	{"mmlu", "gen", "Multiple-choice knowledge, 4 per subject (228)", "cais/mmlu test", "MIT"},
	{"mmlu_pro", "gen", "Harder 10-option knowledge, 10 per category (140)", "TIGER-Lab/MMLU-Pro test", "MIT"},
	{"truthfulqa", "gen", "Avoiding common misconceptions, mc1, shuffled (150)", "truthfulqa/truthful_qa validation", "Apache-2.0"},
	{"ifeval", "gen", "Verifiable instruction following, strict (200)", "google/IFEval", "Apache-2.0"},
	{"generated", "gen", "Seeded arithmetic, counting, dates, logic, JSON and Mermaid (225)", "original (generated)", "MIT"},
}

// Find returns a config by name.
func Find(name string) (Config, bool) {
	for _, c := range Configs {
		if c.Name == name {
			return c, true
		}
	}
	return Config{}, false
}

// Names returns the config names of a kind ("" for all).
func Names(kind string) []string {
	var out []string
	for _, c := range Configs {
		if kind == "" || c.Kind == kind {
			out = append(out, c.Name)
		}
	}
	return out
}

// Profile limits how many items per config a run uses.
type Profile struct {
	Name   string
	Decide int // cases per workflow
	Gen    int // items per config, answer track
	Reason int // items per config, reason track (it is many times slower)
}

// ReasonSubset is how many items per generative config the standard reason track uses; the
// HF dataset publishes exactly these as the reason_subset config.
const ReasonSubset = 40

// Profiles: quick for a smoke run, standard for the leaderboard, full for everything.
var Profiles = map[string]Profile{
	"quick":    {"quick", 10, 20, 10},
	"standard": {"standard", 0, 0, ReasonSubset}, // the whole test split; reason: the published subset
	"full":     {"full", 0, 0, 0},
}

// GenItems returns the System Two items of a config. seed only matters for `generated`
// (0 means the published seed).
func GenItems(config string, seed int64) ([]schema.GenItem, error) {
	if config == "generated" {
		if seed == 0 {
			seed = PublishedSeed
		}
		return Generated(seed), nil
	}
	f, err := dataFS.Open("data/" + config + ".jsonl.gz")
	if err != nil {
		return nil, fmt.Errorf("unknown task set %q", config)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var out []schema.GenItem
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var it schema.GenItem
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, sc.Err()
}

// DecideItems returns the System One cases of a workflow and split ("test" or "train").
func DecideItems(workflow, split string) ([]schema.DecideItem, error) {
	w, ok := workflows[workflow]
	if !ok {
		return nil, fmt.Errorf("unknown workflow %q", workflow)
	}
	n, base := 100, int64(1000)
	if split == "train" {
		n, base = 300, 2000
	}
	out := make([]schema.DecideItem, n)
	for i := range out {
		out[i] = w(newRand(workflow, base+int64(i)))
		out[i].ID = fmt.Sprintf("%s_%s_%04d", workflow, split, i)
		out[i].Workflow = workflow
		out[i].Split = split
	}
	return out, nil
}

// Prior returns the mean train gold distribution of each question of a workflow: the
// "ignores the input" reference row.
func Prior(workflow string) map[string]map[string]float64 {
	items, _ := DecideItems(workflow, "train")
	sum := map[string]map[string]float64{}
	for _, it := range items {
		for q, g := range it.Gold {
			if sum[q] == nil {
				sum[q] = map[string]float64{}
			}
			for k, v := range g.Probabilities {
				sum[q][k] += v / float64(len(items))
			}
		}
	}
	return sum
}

func sortedStrings(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Spread keeps n items spread evenly over the list (0 = all). Runs and the published
// reason_subset config both use it, so they hold the same items.
func Spread[T any](xs []T, n int) []T {
	if n <= 0 || n >= len(xs) {
		return xs
	}
	out := make([]T, n)
	for i := range out {
		out[i] = xs[i*len(xs)/n]
	}
	return out
}
