package score

import (
	"encoding/json"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

const letters = "ABCDEFGHIJ"

var (
	reThinkBlock  = regexp.MustCompile(`(?s)<think>.*?</think>`)
	reAnswerMC    = regexp.MustCompile(`(?i)answer\s*(?:is)?\s*[:：]?\s*\**\(?([A-J])\)?\**(?:[^A-Za-z]|$)`)
	reLeadMC      = regexp.MustCompile(`^\**\(?([A-J])\)?\**(?:[.):]|\s*$)`)
	reAnswerNum   = regexp.MustCompile(`(?i)answer\s*(?:is)?\s*[:：]?\s*\**\s*\$?\s*(-?[\d,]*\.?\d+)`)
	reNumber      = regexp.MustCompile(`-?[\d,]*\.?\d+`)
	reAnswerLine  = regexp.MustCompile(`(?im)^\s*\**answer\**\s*[:：]\s*(.+?)\s*$`)
	reCommaSpace  = regexp.MustCompile(`\s*,\s*`)
	reFencedBlock = regexp.MustCompile("(?s)```[a-zA-Z]*\\s*\\n(.*?)```")
)

// StripThinking removes reasoning so only the final answer is scored. A <think> block that
// never closes means the model ran out of budget while thinking: the answer is empty.
func StripThinking(s string) (answer string, truncated bool) {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		return strings.TrimSpace(s[i+len("</think>"):]), false
	}
	if strings.Contains(s, "<think>") {
		return "", true
	}
	return strings.TrimSpace(reThinkBlock.ReplaceAllString(s, "")), false
}

// Extract returns the model's final answer for an item, in the form CheckAnswer compares.
func Extract(it schema.GenItem, output string) string {
	text, _ := StripThinking(output)
	switch it.AnswerType {
	case "mc":
		return extractMC(text, len(it.Choices), it.Choices)
	case "number":
		return extractNumber(text)
	case "exact":
		if m := reAnswerLine.FindAllStringSubmatch(text, -1); len(m) > 0 {
			return normText(m[len(m)-1][1])
		}
		lines := strings.Split(strings.TrimSpace(text), "\n")
		return normText(lines[len(lines)-1])
	default:
		return text
	}
}

func extractMC(text string, n int, choices []string) string {
	valid := func(l string) bool { return l != "" && strings.Index(letters, l) < n }
	if m := reAnswerMC.FindAllStringSubmatch(text, -1); len(m) > 0 {
		if l := m[len(m)-1][1]; valid(l) {
			return l
		}
	}
	t := strings.TrimSpace(text)
	if m := reLeadMC.FindStringSubmatch(t); m != nil && valid(m[1]) {
		return m[1]
	}
	low := strings.ToLower(t)
	found := ""
	for i, c := range choices {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && strings.Contains(low, c) {
			if found != "" {
				return "" // more than one option named
			}
			found = string(letters[i])
		}
	}
	return found
}

func extractNumber(text string) string {
	if m := reAnswerNum.FindAllStringSubmatch(text, -1); len(m) > 0 {
		return normNumber(m[len(m)-1][1])
	}
	if m := reNumber.FindAllString(text, -1); len(m) > 0 {
		return normNumber(m[len(m)-1])
	}
	return ""
}

func normNumber(s string) string {
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func normText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, " .!*\"'`")
	s = strings.Join(strings.Fields(s), " ")
	return reCommaSpace.ReplaceAllString(s, ", ")
}

// CheckAnswer reports whether the output answers the item correctly.
func CheckAnswer(it schema.GenItem, output string) (correct bool, extracted string) {
	text, _ := StripThinking(output)
	switch it.AnswerType {
	case "mc":
		e := Extract(it, output)
		return e == it.Answer, e
	case "number":
		e := Extract(it, output)
		a, err1 := strconv.ParseFloat(e, 64)
		b, err2 := strconv.ParseFloat(normNumber(it.Answer), 64)
		return err1 == nil && err2 == nil && math.Abs(a-b) < 1e-6, e
	case "exact":
		e := Extract(it, output)
		if e == normText(it.Answer) {
			return true, e
		}
		for _, alt := range stringList(it.Meta["accept"]) {
			if e == normText(alt) {
				return true, e
			}
		}
		return false, e
	case "ifeval":
		return CheckIFEval(it, text), ""
	case "json":
		return checkJSON(it, text), ""
	case "mermaid":
		return checkMermaid(it, text), ""
	}
	return false, ""
}

func stringList(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = x
	}
	return out
}

// firstJSONObject finds the first parseable {...} in text (fenced or bare).
func firstJSONObject(text string) (map[string]any, bool) {
	cands := []string{}
	for _, m := range reFencedBlock.FindAllStringSubmatch(text, -1) {
		cands = append(cands, m[1])
	}
	cands = append(cands, text)
	for _, c := range cands {
		start := strings.Index(c, "{")
		for start >= 0 {
			depth := 0
			for i := start; i < len(c); i++ {
				switch c[i] {
				case '{':
					depth++
				case '}':
					depth--
				}
				if depth == 0 {
					var m map[string]any
					if json.Unmarshal([]byte(c[start:i+1]), &m) == nil {
						return m, true
					}
					break
				}
			}
			next := strings.Index(c[start+1:], "{")
			if next < 0 {
				break
			}
			start += next + 1
		}
	}
	return nil, false
}

// checkJSON: Meta.values maps each required key to its expected value.
func checkJSON(it schema.GenItem, text string) bool {
	obj, ok := firstJSONObject(text)
	if !ok {
		return false
	}
	want, _ := it.Meta["values"].(map[string]any)
	for k, v := range want {
		got, ok := obj[k]
		if !ok || !jsonEqual(got, v) {
			return false
		}
	}
	return true
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ab) == string(bb) {
		return true
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	return aok && bok && normText(as) == normText(bs)
}

var (
	reMermaidHead = regexp.MustCompile(`^(?:flowchart|graph)\s+(?:TD|TB|LR|RL|BT)\s*;?$`)
	reMermaidEdge = regexp.MustCompile(`([A-Za-z0-9_]+)\s*(?:\[[^\]]*\]|\([^)]*\)|\{[^}]*\})?\s*(?:-->|---|-\.->|==>)\s*(?:\|[^|]*\|\s*)?([A-Za-z0-9_]+)`)
	reMermaidNode = regexp.MustCompile(`([A-Za-z0-9_]+)\s*(?:\[([^\]]*)\]|\(([^)]*)\)|\{([^}]*)\})`)
)

// checkMermaid: the output must hold a flowchart whose edges include every Meta.edges pair.
// Pairs are matched by node label (case-insensitive) or node id.
func checkMermaid(it schema.GenItem, text string) bool {
	block := text
	if m := reFencedBlock.FindStringSubmatch(text); m != nil {
		block = m[1]
	}
	lines := []string{}
	for _, l := range strings.Split(block, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "%%") {
			lines = append(lines, l)
		}
	}
	head := -1
	for i, l := range lines {
		if reMermaidHead.MatchString(l) {
			head = i
			break
		}
	}
	if head < 0 {
		return false
	}
	label := map[string]string{}
	edges := map[[2]string]bool{}
	for _, l := range lines[head+1:] {
		for _, m := range reMermaidNode.FindAllStringSubmatch(l, -1) {
			for _, g := range m[2:] {
				if g != "" {
					label[m[1]] = normText(strings.Trim(g, "\"'"))
				}
			}
		}
		for _, m := range reMermaidEdge.FindAllStringSubmatch(l, -1) {
			edges[[2]string{m[1], m[2]}] = true
		}
	}
	name := func(id string) string {
		if l, ok := label[id]; ok && l != "" {
			return l
		}
		return normText(id)
	}
	have := map[[2]string]bool{}
	for e := range edges {
		have[[2]string{name(e[0]), name(e[1])}] = true
	}
	want, _ := it.Meta["edges"].([]any)
	for _, w := range want {
		pair, _ := w.([]any)
		if len(pair) != 2 {
			return false
		}
		a, _ := pair[0].(string)
		b, _ := pair[1].(string)
		if !have[[2]string{normText(a), normText(b)}] {
			return false
		}
	}
	return len(want) > 0
}

// BootstrapCI returns the 95% percentile interval of the mean of xs (fixed seed, 1000 draws).
func BootstrapCI(xs []float64) (lo, hi float64) {
	n := len(xs)
	if n == 0 {
		return 0, 0
	}
	r := rand.New(rand.NewSource(1))
	means := make([]float64, 1000)
	for b := range means {
		var s float64
		for i := 0; i < n; i++ {
			s += xs[r.Intn(n)]
		}
		means[b] = s / float64(n)
	}
	sort.Float64s(means)
	return means[25], means[974]
}
