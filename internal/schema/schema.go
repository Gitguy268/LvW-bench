// Package schema holds the task, answer and result types shared by every part of LvW-bench.
package schema

import "encoding/json"

// Tracks.
const (
	TrackDecide = "decide" // System 1: typed decisions scored on probabilities
	TrackAnswer = "answer" // System 2: direct answer, small budget
	TrackReason = "reason" // System 2: reasoning allowed, large budget
)

// AllTracks in display order.
var AllTracks = []string{TrackDecide, TrackAnswer, TrackReason}

// Question is one typed System One question, the same shape as the TypeSafe / llama.cpp
// /v1/systemone API and LocalLLaMA/typed-decisions.
//
//	choice: Criteria is map[option]description
//	score:  Criteria is []level description, lowest first
//	noul:   Criteria is optional map {"true": ..., "false": ...}
type Question struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

// Options returns the answer keys of a question in a stable order.
func (q Question) Options() []string {
	switch q.Type {
	case "noul":
		return []string{"true", "false"}
	case "score":
		var levels []string
		_ = json.Unmarshal(q.Criteria, &levels)
		out := make([]string, len(levels))
		for i := range levels {
			out[i] = itoa(i)
		}
		return out
	default:
		var m orderedMap
		_ = json.Unmarshal(q.Criteria, &m)
		return m.keys
	}
}

// Descriptions maps each option key to its description ("" when absent).
func (q Question) Descriptions() map[string]string {
	out := map[string]string{}
	switch q.Type {
	case "score":
		var levels []string
		_ = json.Unmarshal(q.Criteria, &levels)
		for i, l := range levels {
			out[itoa(i)] = l
		}
	default:
		var m map[string]*string
		_ = json.Unmarshal(q.Criteria, &m)
		for k, v := range m {
			if v != nil {
				out[k] = *v
			}
		}
	}
	return out
}

// Answer is a distribution over a question's options.
type Answer struct {
	Type          string             `json:"type"`
	Label         string             `json:"label,omitempty"`
	Probabilities map[string]float64 `json:"probabilities"`
	Score         *float64           `json:"score,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
}

// DecideItem is one System One case: a state and several typed questions.
type DecideItem struct {
	ID        string              `json:"id"`
	Workflow  string              `json:"workflow"`
	Split     string              `json:"split"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]Question `json:"questions"`
	Gold      map[string]Answer   `json:"gold"`
	Factors   map[string]any      `json:"factors,omitempty"`
	QOrder    []string            `json:"-"`
}

// QuestionIDs returns the question ids in a stable order.
func (d DecideItem) QuestionIDs() []string {
	if len(d.QOrder) > 0 {
		return d.QOrder
	}
	return sortedKeys(d.Questions)
}

// GenItem is one System Two item: a prompt with a checkable answer.
type GenItem struct {
	ID         string         `json:"id"`
	Config     string         `json:"config"`
	Prompt     string         `json:"prompt"`
	Choices    []string       `json:"choices"`
	Answer     string         `json:"answer"`
	AnswerType string         `json:"answer_type"` // mc, number, exact, ifeval, json, mermaid
	Meta       map[string]any `json:"meta"`
	Source     string         `json:"source"`
	License    string         `json:"license"`
}
