package score

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// IFEvalSupported lists the IFEval instructions LvW-bench verifies. The checks follow
// google-research/instruction_following_eval (Apache-2.0), with keywords matched literally.
var IFEvalSupported = map[string]bool{
	"punctuation:no_comma": true, "change_case:english_lowercase": true, "change_case:english_capital": true,
	"length_constraints:number_words": true, "length_constraints:number_paragraphs": true,
	"detectable_format:number_bullet_lists": true, "detectable_format:json_format": true,
	"detectable_format:title": true, "detectable_format:number_highlighted_sections": true,
	"keywords:existence": true, "keywords:forbidden_words": true, "keywords:frequency": true,
	"keywords:letter_frequency": true, "startend:end_checker": true, "startend:quotation": true,
	"detectable_content:postscript": true, "detectable_content:number_placeholders": true,
}

var (
	reWord        = regexp.MustCompile(`\w+`)
	reParaSplit   = regexp.MustCompile(`\s?\*\*\*\s?`)
	reBulletStar  = regexp.MustCompile(`(?m)^\s*\*[^\*].*$`)
	reBulletDash  = regexp.MustCompile(`(?m)^\s*-.*$`)
	reTitle       = regexp.MustCompile(`<<[^\n]+>>`)
	reHighlight   = regexp.MustCompile(`\*[^\n\*]*\*`)
	reDoubleHi    = regexp.MustCompile(`\*\*[^\n\*]*\*\*`)
	rePlaceholder = regexp.MustCompile(`\[.*?\]`)
)

// CheckIFEval is strict prompt-level accuracy: every instruction must pass.
func CheckIFEval(it schema.GenItem, text string) bool {
	ids := stringList(it.Meta["instruction_id_list"])
	kws, _ := it.Meta["kwargs"].([]any)
	if len(ids) == 0 {
		return false
	}
	for i, id := range ids {
		kw := map[string]any{}
		if i < len(kws) {
			kw, _ = kws[i].(map[string]any)
		}
		if !checkInstruction(id, kw, text) {
			return false
		}
	}
	return true
}

func num(kw map[string]any, k string) int {
	switch v := kw[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}

func str(kw map[string]any, k string) string { s, _ := kw[k].(string); return s }

func relation(rel string, actual, want int) bool {
	if rel == "less than" {
		return actual < want
	}
	return actual >= want // "at least"
}

func hasCased(s string, upper bool) bool {
	cased := false
	for _, r := range s {
		if unicode.IsLetter(r) && unicode.ToUpper(r) != unicode.ToLower(r) {
			cased = true
			if upper && unicode.IsLower(r) || !upper && unicode.IsUpper(r) {
				return false
			}
		}
	}
	return cased
}

func checkInstruction(id string, kw map[string]any, v string) bool {
	switch id {
	case "punctuation:no_comma":
		return !strings.Contains(v, ",")
	case "change_case:english_lowercase":
		return hasCased(v, false)
	case "change_case:english_capital":
		return hasCased(v, true)
	case "length_constraints:number_words":
		return relation(str(kw, "relation"), len(reWord.FindAllString(v, -1)), num(kw, "num_words"))
	case "length_constraints:number_paragraphs":
		paras := reParaSplit.Split(v, -1)
		n := len(paras)
		for i, p := range paras {
			if strings.TrimSpace(p) == "" {
				if i == 0 || i == len(paras)-1 {
					n--
				} else {
					return false
				}
			}
		}
		return n == num(kw, "num_paragraphs")
	case "detectable_format:number_bullet_lists":
		return len(reBulletStar.FindAllString(v, -1))+len(reBulletDash.FindAllString(v, -1)) == num(kw, "num_bullets")
	case "detectable_format:json_format":
		s := strings.TrimSpace(v)
		for _, p := range []string{"```json", "```Json", "```JSON", "```"} {
			if strings.HasPrefix(s, p) {
				s = s[len(p):]
				break
			}
		}
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
		var x any
		return json.Unmarshal([]byte(s), &x) == nil
	case "detectable_format:title":
		for _, t := range reTitle.FindAllString(v, -1) {
			if strings.TrimSpace(strings.Trim(t, "<>")) != "" {
				return true
			}
		}
		return false
	case "detectable_format:number_highlighted_sections":
		n := 0
		for _, h := range reHighlight.FindAllString(v, -1) {
			if strings.TrimSpace(strings.Trim(h, "*")) != "" {
				n++
			}
		}
		for _, h := range reDoubleHi.FindAllString(v, -1) {
			if strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(h, "**"), "**")) != "" {
				n++
			}
		}
		return n >= num(kw, "num_highlights")
	case "keywords:existence":
		for _, k := range stringList(kw["keywords"]) {
			if !regexp.MustCompile(`(?i)` + regexp.QuoteMeta(k)).MatchString(v) {
				return false
			}
		}
		return true
	case "keywords:forbidden_words":
		for _, k := range stringList(kw["forbidden_words"]) {
			if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(k) + `\b`).MatchString(v) {
				return false
			}
		}
		return true
	case "keywords:frequency":
		n := len(regexp.MustCompile(`(?i)`+regexp.QuoteMeta(str(kw, "keyword"))).FindAllString(v, -1))
		return relation(str(kw, "relation"), n, num(kw, "frequency"))
	case "keywords:letter_frequency":
		l := strings.ToLower(str(kw, "letter"))
		n := strings.Count(strings.ToLower(v), l)
		if l == "" {
			return false
		}
		return relation(str(kw, "let_relation"), n, num(kw, "let_frequency"))
	case "startend:end_checker":
		s := strings.ToLower(strings.Trim(strings.TrimSpace(v), `"`))
		return strings.HasSuffix(s, strings.ToLower(strings.TrimSpace(str(kw, "end_phrase"))))
	case "startend:quotation":
		s := strings.TrimSpace(v)
		return len(s) > 1 && s[0] == '"' && s[len(s)-1] == '"'
	case "detectable_content:postscript":
		m := str(kw, "postscript_marker")
		var re *regexp.Regexp
		switch m {
		case "P.P.S":
			re = regexp.MustCompile(`(?m)\s*p\.\s?p\.\s?s.*$`)
		case "P.S.":
			re = regexp.MustCompile(`(?m)\s*p\.\s?s\..*$`)
		default:
			re = regexp.MustCompile(`(?m)\s*` + regexp.QuoteMeta(strings.ToLower(m)) + `.*$`)
		}
		return re.MatchString(strings.ToLower(v))
	case "detectable_content:number_placeholders":
		return len(rePlaceholder.FindAllString(v, -1)) >= num(kw, "num_placeholders")
	}
	return false
}
