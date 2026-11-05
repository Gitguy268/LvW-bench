package tasks

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// Generated builds the `generated` config: 25 items in each of 9 families. Every answer is
// computed, so a fresh seed gives fresh items that no model can have memorised.
func Generated(seed int64) []schema.GenItem {
	fams := []struct {
		name string
		fn   func(*rand.Rand) schema.GenItem
	}{
		{"arithmetic", genArithmetic}, {"word_problem", genWordProblem}, {"letter_count", genLetterCount},
		{"sorting", genSorting}, {"weekday", genWeekday}, {"syllogism", genSyllogism},
		{"list_index", genListIndex}, {"json", genJSON}, {"mermaid", genMermaid},
	}
	var out []schema.GenItem
	for _, f := range fams {
		r := newRand("generated/"+f.name, seed)
		for i := 0; i < 25; i++ {
			it := f.fn(r)
			it.ID = fmt.Sprintf("generated_%s_%02d", f.name, i)
			it.Config = "generated"
			if it.Meta == nil {
				it.Meta = map[string]any{}
			}
			it.Meta["family"] = f.name
			it.Meta["seed"] = seed
			it.Source = "original (generated)"
			it.License = "MIT"
			if it.Choices == nil {
				it.Choices = []string{}
			}
			out = append(out, it)
		}
	}
	return out
}

func genArithmetic(r *rand.Rand) schema.GenItem {
	a, b, c := 11+r.Intn(89), 2+r.Intn(18), 5+r.Intn(200)
	switch r.Intn(3) {
	case 0:
		return schema.GenItem{Prompt: fmt.Sprintf("What is %d × %d − %d?", a, b, c), Answer: fmt.Sprint(a*b - c), AnswerType: "number"}
	case 1:
		return schema.GenItem{Prompt: fmt.Sprintf("What is (%d + %d) × %d?", a, c, b), Answer: fmt.Sprint((a + c) * b), AnswerType: "number"}
	}
	return schema.GenItem{Prompt: fmt.Sprintf("What is %d − %d + %d × %d?", c+a, a, b, b), Answer: fmt.Sprint(c + b*b), AnswerType: "number"}
}

var names = []string{"Maya", "Noah", "Ines", "Sem", "Lotte", "Omar", "Yuki", "Finn", "Zara", "Ravi"}

func genWordProblem(r *rand.Rand) schema.GenItem {
	n := pick(r, names)
	start, bags, per, give := 3+r.Intn(20), 2+r.Intn(6), 3+r.Intn(9), 1+r.Intn(10)
	switch r.Intn(3) {
	case 0:
		return schema.GenItem{Prompt: fmt.Sprintf("%s has %d biscuits, buys %d packs with %d biscuits each, then gives %d to Spot the dog. How many biscuits does %s have now?",
			n, start, bags, per, give, n), Answer: fmt.Sprint(start + bags*per - give), AnswerType: "number"}
	case 1:
		price, qty, paid := 2+r.Intn(8), 2+r.Intn(5), 50
		return schema.GenItem{Prompt: fmt.Sprintf("%s buys %d notebooks at %d euros each and pays with a %d euro note. How much change, in euros, does %s get?",
			n, qty, price, paid, n), Answer: fmt.Sprint(paid - qty*price), AnswerType: "number"}
	}
	speed, hours, mins := 40+5*r.Intn(10), 1+r.Intn(4), pick(r, []int{0, 30})
	dist := speed*hours + speed*mins/60
	return schema.GenItem{Prompt: fmt.Sprintf("A train travels at %d km per hour for %d hours and %d minutes. How many kilometres does it travel?",
		speed, hours, mins), Answer: fmt.Sprint(dist), AnswerType: "number"}
}

var words = []string{"strawberry", "banana", "bookkeeper", "mississippi", "occurrence", "committee", "parallel",
	"balloon", "assessment", "coffee", "programming", "letter", "success", "tomorrow", "possession", "barrel"}

func genLetterCount(r *rand.Rand) schema.GenItem {
	w := pick(r, words)
	l := string(w[r.Intn(len(w))])
	return schema.GenItem{Prompt: fmt.Sprintf("How many times does the letter \"%s\" appear in the word \"%s\"?", l, w),
		Answer: fmt.Sprint(strings.Count(w, l)), AnswerType: "number"}
}

var nouns = []string{"maple", "anchor", "violet", "copper", "lantern", "meadow", "pepper", "harbor", "cactus",
	"walnut", "orbit", "falcon", "ember", "glacier", "kettle", "biscuit", "tulip", "quartz", "saddle", "dune"}

func genSorting(r *rand.Rand) schema.GenItem {
	ws := append([]string{}, nouns...)
	r.Shuffle(len(ws), func(i, j int) { ws[i], ws[j] = ws[j], ws[i] })
	ws = ws[:4+r.Intn(3)]
	s := append([]string{}, ws...)
	sort.Strings(s)
	return schema.GenItem{Prompt: fmt.Sprintf("Sort these words alphabetically: %s. Answer with the sorted words separated by commas.",
		strings.Join(ws, ", ")), Answer: strings.Join(s, ", "), AnswerType: "exact"}
}

func genWeekday(r *rand.Rand) schema.GenItem {
	days := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	if r.Intn(2) == 0 {
		d, n := r.Intn(7), 2+r.Intn(60)
		return schema.GenItem{Prompt: fmt.Sprintf("Today is %s. What day of the week will it be in %d days?", days[d], n),
			Answer: days[(d+n)%7], AnswerType: "exact"}
	}
	t := time.Date(2026, time.Month(1+r.Intn(12)), 1+r.Intn(28), 0, 0, 0, 0, time.UTC)
	n := 3 + r.Intn(40)
	u := t.AddDate(0, 0, n)
	return schema.GenItem{Prompt: fmt.Sprintf("What date is %d days after %s? Answer in the form YYYY-MM-DD.", n, t.Format("2 January 2006")),
		Answer: u.Format("2006-01-02"), AnswerType: "exact"}
}

var nonsense = []string{"bloops", "razzies", "lazzies", "florbs", "wumps", "gleebs", "snarks", "trobs", "zindles", "quibs"}

func genSyllogism(r *rand.Rand) schema.GenItem {
	ws := append([]string{}, nonsense...)
	r.Shuffle(len(ws), func(i, j int) { ws[i], ws[j] = ws[j], ws[i] })
	a, b, c := ws[0], ws[1], ws[2]
	choices := []string{"Yes", "No", "Cannot be determined"}
	switch r.Intn(4) {
	case 0: // valid chain
		return schema.GenItem{Prompt: fmt.Sprintf("All %s are %s. All %s are %s. Are all %s definitely %s?", a, b, b, c, a, c), Choices: choices, Answer: "A", AnswerType: "mc"}
	case 1: // undistributed middle
		return schema.GenItem{Prompt: fmt.Sprintf("All %s are %s. All %s are %s. Are all %s definitely %s?", a, b, c, b, a, c), Choices: choices, Answer: "C", AnswerType: "mc"}
	case 2: // contradiction
		return schema.GenItem{Prompt: fmt.Sprintf("All %s are %s. No %s are %s. Can any %s be %s?", a, b, b, c, a, c), Choices: choices, Answer: "B", AnswerType: "mc"}
	}
	return schema.GenItem{Prompt: fmt.Sprintf("Some %s are %s. All %s are %s. Are some %s definitely %s?", a, b, b, c, a, c), Choices: choices, Answer: "A", AnswerType: "mc"}
}

func genListIndex(r *rand.Rand) schema.GenItem {
	ws := append([]string{}, nouns...)
	r.Shuffle(len(ws), func(i, j int) { ws[i], ws[j] = ws[j], ws[i] })
	ws = ws[:5+r.Intn(4)]
	ord := []string{"first", "second", "third", "fourth", "fifth"}
	i := r.Intn(len(ord))
	if r.Intn(2) == 0 {
		return schema.GenItem{Prompt: fmt.Sprintf("Here is a list: %s. What is the %s item?", strings.Join(ws, ", "), ord[i]), Answer: ws[i], AnswerType: "exact"}
	}
	return schema.GenItem{Prompt: fmt.Sprintf("Here is a list: %s. What is the last item?", strings.Join(ws, ", ")), Answer: ws[len(ws)-1], AnswerType: "exact"}
}

func genJSON(r *rand.Rand) schema.GenItem {
	n := pick(r, names)
	age := 18 + r.Intn(60)
	city := pick(r, []string{"Amsterdam", "Lagos", "Osaka", "Lima", "Oslo", "Utrecht", "Porto"})
	pet := pick(r, []string{"dog", "cat", "none"})
	vals := map[string]any{"name": n, "age": float64(age), "city": city}
	prompt := fmt.Sprintf("Return a JSON object with the key \"name\" set to \"%s\", \"age\" set to the number %d and \"city\" set to \"%s\"", n, age, city)
	if r.Intn(2) == 0 {
		vals["pet"] = pet
		prompt += fmt.Sprintf(", and \"pet\" set to \"%s\"", pet)
	}
	return schema.GenItem{Prompt: prompt + ". Output only the JSON.", AnswerType: "json", Meta: map[string]any{"values": vals}}
}

func genMermaid(r *rand.Rand) schema.GenItem {
	steps := [][]string{
		{"Start", "Fill kettle", "Boil water", "Pour tea", "Done"},
		{"Open app", "Log in", "Pick song", "Play", "Stop"},
		{"Wake up", "Walk Spot", "Breakfast", "Work"},
		{"Order", "Pay", "Pack", "Ship", "Deliver"},
		{"Idea", "Sketch", "Build", "Test", "Release"},
		{"Request", "Review", "Approve", "Merge"},
	}
	s := pick(r, steps)
	k := 3 + r.Intn(len(s)-2)
	s = s[:k]
	var edges []any
	var parts []string
	for i := 0; i+1 < len(s); i++ {
		edges = append(edges, []any{s[i], s[i+1]})
		parts = append(parts, fmt.Sprintf("\"%s\" leads to \"%s\"", s[i], s[i+1]))
	}
	return schema.GenItem{Prompt: "Write a Mermaid flowchart (start with `flowchart TD`) where " + strings.Join(parts, ", ") +
		". Use the quoted text as the node labels. Output only the Mermaid code.", AnswerType: "mermaid", Meta: map[string]any{"edges": edges}}
}
