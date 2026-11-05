package tasks

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"math"
	"math/rand"
	"strconv"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

func newRand(name string, seed int64) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(name))
	return rand.New(rand.NewSource(int64(h.Sum64()>>1) ^ seed))
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.Intn(len(xs))] }

// opt is one option of a choice question, in display order.
type opt struct{ key, desc string }

func choiceQ(instr string, opts ...opt) schema.Question {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, o := range opts {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(o.key)
		d, _ := json.Marshal(o.desc)
		b.Write(k)
		b.WriteByte(':')
		b.Write(d)
	}
	b.WriteByte('}')
	return schema.Question{Type: "choice", Instructions: instr, Criteria: b.Bytes()}
}

func scoreQ(instr string, levels ...string) schema.Question {
	c, _ := json.Marshal(levels)
	return schema.Question{Type: "score", Instructions: instr, Criteria: c}
}

func noulQ(instr, whenTrue, whenFalse string) schema.Question {
	c, _ := json.Marshal(map[string]string{"true": whenTrue, "false": whenFalse})
	return schema.Question{Type: "noul", Instructions: instr, Criteria: c}
}

func round6(x float64) float64 { return math.Round(x*1e6) / 1e6 }

// dist turns weights into a gold answer (normalised, rounded to 6 places).
func dist(typ string, w map[string]float64) schema.Answer {
	var s float64
	for _, v := range w {
		s += v
	}
	p := map[string]float64{}
	best, bestV := "", -1.0
	for _, k := range sortedStrings(w) {
		p[k] = round6(w[k] / s)
		if p[k] > bestV {
			best, bestV = k, p[k]
		}
	}
	a := schema.Answer{Type: typ, Label: best, Probabilities: p, Confidence: bestV}
	if typ == "score" {
		var e float64
		for k, v := range p {
			n, _ := strconv.Atoi(k)
			e += float64(n) * v
		}
		e = round6(e)
		a.Score = &e
	}
	return a
}

// softmax gold over choice scores: higher score, more mass; temp sets how soft.
func softmax(scores map[string]float64, temp float64) schema.Answer {
	w := map[string]float64{}
	mx := math.Inf(-1)
	for _, v := range scores {
		mx = math.Max(mx, v)
	}
	for k, v := range scores {
		w[k] = math.Exp((v - mx) / temp)
	}
	return dist("choice", w)
}

// level gold for a score question: mass p on the level, the rest split over neighbours.
func level(l, n int, p float64) schema.Answer {
	w := map[string]float64{}
	for i := 0; i < n; i++ {
		w[strconv.Itoa(i)] = 0.002
	}
	w[strconv.Itoa(l)] = p
	rest := 1 - p
	var nb []int
	for _, d := range []int{-1, 1} {
		if l+d >= 0 && l+d < n {
			nb = append(nb, l+d)
		}
	}
	for _, j := range nb {
		w[strconv.Itoa(j)] += rest / float64(len(nb))
	}
	return dist("score", w)
}

// yes gold for a noul question.
func yes(p float64) schema.Answer {
	return dist("noul", map[string]float64{"true": p, "false": 1 - p})
}

func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
