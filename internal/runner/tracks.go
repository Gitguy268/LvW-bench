package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Gitguy268/LvW-bench/internal/backend"
	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/schema"
	"github.com/Gitguy268/LvW-bench/internal/score"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

var (
	off = false
	on  = true
)

// runDecide runs the System One track.
func runDecide(ctx context.Context, o *Options, c *backend.Client, mods []string, prof tasks.Profile) (*results.Track, []results.Prediction, error) {
	adapter := o.Adapter
	if adapter == "" || adapter == "auto" {
		adapter = "logprobs"
		if backend.Has(mods, "decisions") {
			adapter = "systemone"
		}
	}
	n := prof.Decide
	if o.Limit > 0 {
		n = o.Limit
	}
	type job struct {
		cfg string
		it  schema.DecideItem
	}
	var jobs []job
	for _, wf := range configsFor(o, "decide") {
		items, err := tasks.DecideItems(wf, "test")
		if err != nil {
			return nil, nil, err
		}
		for _, it := range pickN(items, n) {
			jobs = append(jobs, job{wf, it})
		}
	}
	if len(jobs) == 0 {
		return nil, nil, nil
	}
	// Probe once: does this server return logprobs? If not, fall back to sampling.
	if adapter == "logprobs" {
		p, _ := decidePrompt(jobs[0].it.State, jobs[0].it.Questions[jobs[0].it.QuestionIDs()[0]])
		r, err := c.Chat(ctx, backend.ChatRequest{Messages: []backend.Message{{Role: "user", Content: p}}, MaxTokens: 1, TopLogprobs: 20, Thinking: &off})
		if err != nil {
			return nil, nil, fmt.Errorf("probe: %w", err)
		}
		if !r.HasLogprobs {
			adapter = "sampled"
			o.emit(Event{Kind: "status", Track: schema.TrackDecide, Msg: "server returns no logprobs: sampling 8 answers per question instead"})
		}
	}
	o.emit(Event{Kind: "status", Track: schema.TrackDecide, Msg: "adapter: " + adapter})

	var mu sync.Mutex
	var preds []results.Prediction
	scores := map[string][]score.DecisionScore{}
	var all []score.DecisionScore
	var lat []float64
	answered, asked, errs, done, correct := 0, 0, 0, 0, 0
	totalQ := 0
	for _, j := range jobs {
		totalQ += len(j.it.Questions)
	}
	start := time.Now()
	pool(ctx, len(jobs), o.Parallel, func(i int) {
		j := jobs[i]
		t0 := time.Now()
		dists, err := decideCase(ctx, c, adapter, j.it)
		ms := float64(time.Since(t0).Microseconds()) / 1000
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs++
		}
		lat = append(lat, ms)
		for _, qid := range j.it.QuestionIDs() {
			q := j.it.Questions[qid]
			asked++
			pd, ok := dists[qid]
			if ok && len(pd) > 0 {
				answered++
			}
			s := score.ScoreDecision(q, pd, j.it.Gold[qid].Probabilities)
			scores[j.cfg] = append(scores[j.cfg], s)
			all = append(all, s)
			if s.Correct {
				correct++
			}
			p := results.Prediction{Track: schema.TrackDecide, Config: j.cfg, ID: j.it.ID, Question: qid, Type: q.Type,
				Probabilities: s.Normalized, Correct: s.Correct, Millis: ms}
			if err != nil {
				p.Error = err.Error()
			}
			preds = append(preds, p)
		}
		done += len(j.it.Questions)
		o.emit(Event{Kind: "progress", Track: schema.TrackDecide, Config: j.cfg, Done: done, Total: totalQ, Correct: correct})
	})
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if errs == len(jobs) {
		return nil, nil, fmt.Errorf("every request failed: %s", firstError(preds))
	}
	sum := score.SummarizeDecisions(all)
	t := &results.Track{Accuracy: round(sum.Accuracy, 4), KL: round(sum.KL, 4), Brier: round(sum.Brier, 4), ECE: round(sum.ECE, 4),
		N: sum.N, Adapter: adapter, P50Millis: round(median(lat), 1), WallSeconds: round(time.Since(start).Seconds(), 1),
		Coverage: round(float64(answered)/float64(asked), 4), Errors: errs, PerConfig: map[string]results.ConfigScore{}}
	xs := make([]float64, len(all))
	for i, s := range all {
		if s.Correct {
			xs[i] = 1
		}
	}
	t.CILow, t.CIHigh = score.BootstrapCI(xs)
	t.CILow, t.CIHigh = round(t.CILow, 4), round(t.CIHigh, 4)
	for cfg, ss := range scores {
		s := score.SummarizeDecisions(ss)
		t.PerConfig[cfg] = results.ConfigScore{N: s.N, Accuracy: round(s.Accuracy, 4), KL: round(s.KL, 4), Brier: round(s.Brier, 4), ECE: round(s.ECE, 4)}
	}
	return t, preds, nil
}

func firstError(p []results.Prediction) string {
	for _, x := range p {
		if x.Error != "" {
			return x.Error
		}
	}
	return ""
}

// decideCase answers every question of one case with the chosen adapter.
func decideCase(ctx context.Context, c *backend.Client, adapter string, it schema.DecideItem) (map[string]map[string]float64, error) {
	if adapter == "systemone" {
		d, _, err := c.SystemOne(ctx, it.State, it.Questions)
		return d, err
	}
	out := map[string]map[string]float64{}
	var firstErr error
	for _, qid := range it.QuestionIDs() {
		q := it.Questions[qid]
		prompt, labels := decidePrompt(it.State, q)
		opts := q.Options()
		msgs := []backend.Message{{Role: "user", Content: prompt}}
		if adapter == "sampled" {
			votes := map[string]float64{}
			for k := 0; k < 8; k++ {
				r, err := c.Chat(ctx, backend.ChatRequest{Messages: msgs, MaxTokens: 4, Temperature: 0.7, Thinking: &off, Seed: k + 1})
				if err != nil {
					firstErr = err
					break
				}
				votes[strings.TrimSpace(r.Content)] += 1
			}
			if p, ok := labelProbs(votes, labels, opts); ok {
				out[qid] = p
			}
			continue
		}
		r, err := c.Chat(ctx, backend.ChatRequest{Messages: msgs, MaxTokens: 1, TopLogprobs: 20, Thinking: &off})
		if err != nil {
			firstErr = err
			continue
		}
		if p, ok := labelProbs(r.FirstTop, labels, opts); ok {
			out[qid] = p
		}
	}
	return out, firstErr
}

// runGen runs a System Two track (answer or reason).
func runGen(ctx context.Context, o *Options, c *backend.Client, track string, prof tasks.Profile) (*results.Track, []results.Prediction, error) {
	n := prof.Gen
	if track == schema.TrackReason {
		n = prof.Reason
	}
	if o.Limit > 0 {
		n = o.Limit
	}
	var items []schema.GenItem
	for _, cfg := range configsFor(o, "gen") {
		xs, err := tasks.GenItems(cfg, o.Seed)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, pickN(xs, n)...)
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	thinking := &off
	if track == schema.TrackReason {
		thinking = &on
	}
	var mu sync.Mutex
	preds := make([]results.Prediction, len(items))
	var lat []float64
	outTok, truncated, native, errs, done, correct := 0, 0, 0, 0, 0, 0
	start := time.Now()
	pool(ctx, len(items), o.Parallel, func(i int) {
		it := items[i]
		req := backend.ChatRequest{Messages: []backend.Message{{Role: "user", Content: genPrompt(it, track)}},
			MaxTokens: maxTokens(it, track, o.ReasonBudget), Thinking: thinking}
		r, err := c.Chat(ctx, req)
		p := results.Prediction{Track: track, Config: it.Config, ID: it.ID}
		if err != nil {
			p.Error = err.Error()
		} else {
			full := r.Content
			if r.Reasoning != "" {
				full = "<think>" + r.Reasoning + "</think>" + r.Content
			}
			_, cut := score.StripThinking(full)
			p.Truncated = cut || (r.FinishReason == "length" && strings.TrimSpace(r.Content) == "")
			p.Correct, p.Extracted = score.CheckAnswer(it, full)
			if p.Extracted == "" && it.AnswerType != "ifeval" && it.AnswerType != "json" && it.AnswerType != "mermaid" {
				p.Extracted = score.Extract(it, full)
			}
			// The scored text is kept whole so bench/score.py rescores to the same number;
			// only separated reasoning (never scored) is clipped.
			p.Output = r.Content
			if r.Reasoning != "" {
				p.Reasoning = clip(r.Reasoning, 4000)
				p.Output = "<think></think>" + r.Content
			}
			p.OutputTokens = r.OutputTokens
			p.Millis = float64(r.Elapsed.Microseconds()) / 1000
		}
		mu.Lock()
		defer mu.Unlock()
		preds[i] = p
		if err != nil {
			errs++
		} else {
			lat = append(lat, p.Millis)
			outTok += p.OutputTokens
			if p.Truncated {
				truncated++
			}
			if r != nil && (r.Reasoning != "" || strings.Contains(r.Content, "<think>")) {
				native++
			}
		}
		if p.Correct {
			correct++
		}
		done++
		o.emit(Event{Kind: "progress", Track: track, Config: it.Config, Done: done, Total: len(items), Correct: correct})
	})
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if errs == len(items) {
		return nil, nil, fmt.Errorf("every request failed: %s", firstError(preds))
	}
	wall := time.Since(start).Seconds()
	per := map[string][]float64{}
	xs := make([]float64, len(preds))
	for i, p := range preds {
		if p.Correct {
			xs[i] = 1
		}
		per[p.Config] = append(per[p.Config], xs[i])
	}
	lo, hi := score.BootstrapCI(xs)
	t := &results.Track{Accuracy: round(mean(xs), 4), CILow: round(lo, 4), CIHigh: round(hi, 4), N: len(xs),
		MeanOutTokens: round(float64(outTok)/float64(max(1, len(items)-errs)), 1), TokensPerSecond: round(float64(outTok)/wall, 1),
		P50Millis: round(median(lat), 1), WallSeconds: round(wall, 1), TruncationRate: round(float64(truncated)/float64(len(items)), 4),
		NativeThinking: track == schema.TrackReason && native > len(items)/4, Errors: errs, PerConfig: map[string]results.ConfigScore{}}
	for cfg, v := range per {
		t.PerConfig[cfg] = results.ConfigScore{N: len(v), Accuracy: round(mean(v), 4)}
	}
	return t, preds, nil
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n/2] + "\n…\n" + s[len(s)-n/2:]
}
