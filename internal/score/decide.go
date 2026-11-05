// Package score holds the LvW-bench scorers. The same rules are implemented in
// web/js/score.js and bench/score.py; testdata/golden_*.json keeps them identical.
package score

import (
	"math"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// DecisionScore is the score of one typed decision.
type DecisionScore struct {
	Correct    bool               `json:"correct"`
	KL         float64            `json:"kl"`
	Brier      float64            `json:"brier"`
	Confidence float64            `json:"confidence"`
	Normalized map[string]float64 `json:"probabilities"`
}

// ScoreDecision compares a predicted distribution with the gold one, following
// LocalLLaMA/typed-decisions bench/score_with_jobs.py (Apache-2.0): options come from the
// question, missing options get 0, the prediction is renormalised (uniform when it sums
// to 0), accuracy is argmax agreement, KL(gold || pred) with a 1e-9 floor, Brier is the
// squared distance summed over options.
func ScoreDecision(q schema.Question, pred, gold map[string]float64) DecisionScore {
	opts := q.Options()
	pred = canonicalNoul(q.Type, pred)
	gold = canonicalNoul(q.Type, gold)
	g := make([]float64, len(opts))
	p := make([]float64, len(opts))
	var sum float64
	for i, o := range opts {
		g[i] = gold[o]
		p[i] = math.Max(pred[o], 0)
		sum += p[i]
	}
	for i := range p {
		if sum > 0 {
			p[i] /= sum
		} else {
			p[i] = 1 / float64(len(p))
		}
	}
	var kl, brier float64
	for i := range opts {
		if g[i] > 0 {
			kl += g[i] * math.Log(g[i]/math.Max(p[i], 1e-9))
		}
		brier += (p[i] - g[i]) * (p[i] - g[i])
	}
	norm := make(map[string]float64, len(opts))
	for i, o := range opts {
		norm[o] = p[i]
	}
	return DecisionScore{
		Correct:    argmax(p) == argmax(g),
		KL:         kl,
		Brier:      brier,
		Confidence: p[argmax(p)],
		Normalized: norm,
	}
}

// canonicalNoul maps yes/no keys to true/false (engines report either).
func canonicalNoul(typ string, m map[string]float64) map[string]float64 {
	if typ != "noul" {
		return m
	}
	out := make(map[string]float64, 2)
	for k, v := range m {
		switch k {
		case "yes", "Yes", "True":
			k = "true"
		case "no", "No", "False":
			k = "false"
		}
		out[k] += v
	}
	return out
}

func argmax(v []float64) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

// DecideSummary aggregates decision scores.
type DecideSummary struct {
	N        int     `json:"n_decisions"`
	Accuracy float64 `json:"accuracy"`
	KL       float64 `json:"kl_from_gold"`
	Brier    float64 `json:"brier"`
	ECE      float64 `json:"ece"`
}

// SummarizeDecisions computes accuracy, mean KL, mean Brier and 10-bin ECE.
func SummarizeDecisions(s []DecisionScore) DecideSummary {
	n := len(s)
	if n == 0 {
		return DecideSummary{}
	}
	var acc, kl, br float64
	const bins = 10
	var hits, total [bins]float64
	var conf [bins]float64
	for _, d := range s {
		if d.Correct {
			acc++
		}
		kl += d.KL
		br += d.Brier
		b := int(d.Confidence * bins)
		if b > bins-1 {
			b = bins - 1
		}
		total[b]++
		conf[b] += d.Confidence
		if d.Correct {
			hits[b]++
		}
	}
	var ece float64
	for b := 0; b < bins; b++ {
		if total[b] > 0 {
			ece += total[b] / float64(n) * math.Abs(hits[b]/total[b]-conf[b]/total[b])
		}
	}
	fn := float64(n)
	return DecideSummary{N: n, Accuracy: acc / fn, KL: kl / fn, Brier: br / fn, ECE: ece}
}
