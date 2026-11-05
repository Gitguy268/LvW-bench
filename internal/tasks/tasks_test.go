package tasks

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/score"
)

func TestDecideItemsValid(t *testing.T) {
	for _, wf := range Names("decide") {
		test, err := DecideItems(wf, "test")
		if err != nil {
			t.Fatal(err)
		}
		train, _ := DecideItems(wf, "train")
		seen := map[string]bool{}
		for _, it := range train {
			seen[string(it.State)] = true
		}
		dupes := 0
		labels := map[string]map[string]int{}
		for _, it := range test {
			if seen[string(it.State)] {
				dupes++
			}
			for q, g := range it.Gold {
				opts := it.Questions[q].Options()
				var s float64
				for _, o := range opts {
					if _, ok := g.Probabilities[o]; !ok {
						t.Fatalf("%s %s: gold misses option %q", it.ID, q, o)
					}
					s += g.Probabilities[o]
				}
				if s < 0.999 || s > 1.001 {
					t.Fatalf("%s %s: gold sums to %f", it.ID, q, s)
				}
				if labels[q] == nil {
					labels[q] = map[string]int{}
				}
				labels[q][g.Label]++
			}
		}
		if dupes > 0 {
			t.Errorf("%s: %d test states also in train", wf, dupes)
		}
		t.Logf("%s labels: %v", wf, labels)
	}
}

func TestGeneratedSelfConsistent(t *testing.T) {
	for _, it := range Generated(PublishedSeed) {
		var out string
		switch it.AnswerType {
		case "json":
			b, _ := json.Marshal(it.Meta["values"])
			out = "```json\n" + string(b) + "\n```"
		case "mermaid":
			out = "flowchart TD\n"
			for i, e := range it.Meta["edges"].([]any) {
				p := e.([]any)
				out += fmt.Sprintf("  n%d[%s] --> n%d[%s]\n", i, p[0], i+1, p[1])
			}
		default:
			out = "Some reasoning first.\nAnswer: " + it.Answer
		}
		if ok, ex := score.CheckAnswer(it, out); !ok {
			t.Errorf("%s: gold answer %q rejected (extracted %q)\n%s", it.ID, it.Answer, ex, it.Prompt)
		}
	}
}

func TestEmbeddedSets(t *testing.T) {
	for _, c := range Names("gen") {
		items, err := GenItems(c, 0)
		if err != nil || len(items) == 0 {
			t.Fatalf("%s: %v (%d items)", c, err, len(items))
		}
		for _, it := range items {
			if it.AnswerType == "mc" && strings.Index("ABCDEFGHIJ", it.Answer) >= len(it.Choices) {
				t.Fatalf("%s: answer out of range", it.ID)
			}
		}
	}
	_ = schema.TrackDecide
}
