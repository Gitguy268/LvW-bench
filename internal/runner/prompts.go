package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// stateText renders a System One state for a prompt: strings as is, anything else as JSON.
func stateText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") == nil {
		return buf.String()
	}
	return string(raw)
}

// decidePrompt asks one typed question so the first output token is the answer label.
// It returns the prompt and the label each option is answered with.
func decidePrompt(state json.RawMessage, q schema.Question) (string, []string) {
	opts := q.Options()
	desc := q.Descriptions()
	var b strings.Builder
	b.WriteString("Read the state and answer the question.\n\nState:\n")
	b.WriteString(stateText(state))
	b.WriteString("\n\n")
	labels := make([]string, len(opts))
	switch q.Type {
	case "noul":
		fmt.Fprintf(&b, "Statement: %s\n\nIs the statement true for this state?\n", q.Instructions)
		labels = []string{"A", "B"}
		fmt.Fprintf(&b, "A) true%s\nB) false%s\n", dash(desc["true"]), dash(desc["false"]))
		b.WriteString("\nReply with A or B only.")
	case "score":
		fmt.Fprintf(&b, "Question: %s\n\nLevels:\n", q.Instructions)
		for i, o := range opts {
			labels[i] = o
			fmt.Fprintf(&b, "%s) %s\n", o, desc[o])
		}
		b.WriteString("\nReply with the level number only.")
	default:
		fmt.Fprintf(&b, "Question: %s\n\nOptions:\n", q.Instructions)
		for i, o := range opts {
			labels[i] = string(letters[i])
			fmt.Fprintf(&b, "%s) %s%s\n", labels[i], o, dash(desc[o]))
		}
		b.WriteString("\nReply with the letter of the best option only.")
	}
	return b.String(), labels
}

func dash(s string) string {
	if s == "" {
		return ""
	}
	return " — " + s
}

// labelProbs turns first-token probabilities into a distribution over options. Tokens are
// matched after trimming spaces, brackets and a trailing ")" or ".".
func labelProbs(top map[string]float64, labels, opts []string) (map[string]float64, bool) {
	idx := map[string]int{}
	for i, l := range labels {
		idx[l] = i
	}
	p := make([]float64, len(opts))
	var mass float64
	for tok, v := range top {
		t := strings.Trim(strings.TrimSpace(tok), "()*.:")
		if i, ok := idx[t]; ok {
			p[i] += v
			mass += v
		}
	}
	out := map[string]float64{}
	for i, o := range opts {
		out[o] = p[i]
	}
	return out, mass > 0
}

// genPrompt builds the System Two prompt of an item for a track.
func genPrompt(it schema.GenItem, track string) string {
	var b strings.Builder
	b.WriteString(it.Prompt)
	if len(it.Choices) > 0 {
		b.WriteString("\n\n")
		for i, c := range it.Choices {
			fmt.Fprintf(&b, "%c) %s\n", letters[i], c)
		}
	}
	short := it.AnswerType == "mc" || it.AnswerType == "number" || it.AnswerType == "exact"
	if !short {
		return strings.TrimSpace(b.String()) // free-form: the item's own instructions
	}
	what := map[string]string{"mc": "the letter of the correct option", "number": "the final number", "exact": "the answer"}[it.AnswerType]
	if track == schema.TrackReason {
		fmt.Fprintf(&b, "\nThink step by step, then give %s on the last line as \"Answer: <answer>\".", what)
	} else {
		fmt.Fprintf(&b, "\nReply with %s only.", what)
	}
	return strings.TrimSpace(b.String())
}

// maxTokens is the output budget per item and track.
func maxTokens(it schema.GenItem, track string, reasonBudget int) int {
	short := it.AnswerType == "mc" || it.AnswerType == "number" || it.AnswerType == "exact"
	switch {
	case track == schema.TrackAnswer && short:
		return 32
	case track == schema.TrackAnswer:
		return 768
	case short:
		return reasonBudget
	default:
		return reasonBudget + 768
	}
}
