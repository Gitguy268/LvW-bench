package score

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// Golden holds scorer fixtures shared with web/js/score.js and bench/score.py.
//
//go:embed golden.json
var GoldenJSON []byte

// GoldenDecision is one decision fixture.
type GoldenDecision struct {
	Question schema.Question    `json:"question"`
	Pred     map[string]float64 `json:"pred"`
	Gold     map[string]float64 `json:"gold"`
	Want     struct {
		Correct    bool    `json:"correct"`
		KL         float64 `json:"kl"`
		Brier      float64 `json:"brier"`
		Confidence float64 `json:"confidence"`
	} `json:"want"`
}

// GoldenGen is one generation fixture.
type GoldenGen struct {
	Item   schema.GenItem `json:"item"`
	Output string         `json:"output"`
	Want   struct {
		Correct   bool   `json:"correct"`
		Extracted string `json:"extracted"`
	} `json:"want"`
}

// Golden is the fixture file.
type Golden struct {
	Decisions []GoldenDecision `json:"decisions"`
	Summary   struct {
		Want DecideSummary `json:"want"`
	} `json:"summary"`
	Gen []GoldenGen `json:"gen"`
}

// SelfTest checks the scorer against the fixtures and returns the failures.
func SelfTest() ([]string, int) {
	var g Golden
	if err := json.Unmarshal(GoldenJSON, &g); err != nil {
		return []string{"golden.json: " + err.Error()}, 0
	}
	var fails []string
	var ds []DecisionScore
	for i, d := range g.Decisions {
		s := ScoreDecision(d.Question, d.Pred, d.Gold)
		ds = append(ds, s)
		if s.Correct != d.Want.Correct || !near(s.KL, d.Want.KL) || !near(s.Brier, d.Want.Brier) || !near(s.Confidence, d.Want.Confidence) {
			fails = append(fails, fmt.Sprintf("decision %d: got %+v want %+v", i, s, d.Want))
		}
	}
	sum := SummarizeDecisions(ds)
	w := g.Summary.Want
	if !near(sum.Accuracy, w.Accuracy) || !near(sum.KL, w.KL) || !near(sum.Brier, w.Brier) || !near(sum.ECE, w.ECE) {
		fails = append(fails, fmt.Sprintf("summary: got %+v want %+v", sum, w))
	}
	for i, x := range g.Gen {
		ok, ex := CheckAnswer(x.Item, x.Output)
		if ok != x.Want.Correct || ex != x.Want.Extracted {
			fails = append(fails, fmt.Sprintf("gen %d (%s): got %v %q want %v %q", i, x.Item.AnswerType, ok, ex, x.Want.Correct, x.Want.Extracted))
		}
	}
	return fails, len(g.Decisions) + len(g.Gen) + 1
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
