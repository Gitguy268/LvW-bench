package tasks

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// Each workflow builds one case from a seeded random source. The gold is computed from the
// case's own facts by fixed rules, so it is reproducible and needs no teacher model. Rules
// that are judgement calls keep a soft gold; facts that can be read off the state are sharp.
var workflows = map[string]func(*rand.Rand) schema.DecideItem{
	"spot_snake":  spotSnake,
	"it_support":  itSupport,
	"delivery":    delivery,
	"code_review": codeReview,
	"calendar":    calendar,
}

func item(state any, factors map[string]any, qs map[string]schema.Question, gold map[string]schema.Answer, order ...string) schema.DecideItem {
	return schema.DecideItem{State: mustJSON(state), Questions: qs, Gold: gold, Factors: factors, QOrder: order}
}

// --- spot_snake --------------------------------------------------------------------------

var opposite = map[string]string{"up": "down", "down": "up", "left": "right", "right": "left"}

// spotSnake follows the Spot's Biscuit Snake game (github.com/Gitguy268/spot-snake): the
// state is the game's own sentence and each safe move's option text is "move: what it does",
// as in Lucasgitguy-co/spot-snake-decisions-v0.1. The move gold is that dataset's teacher:
// the most reachable room, then continuing straight on a tie, remaining ties shared.
func spotSnake(r *rand.Rand) schema.DecideItem {
	w, h := 10+r.Intn(11), 10+r.Intn(11)
	length := 3 + r.Intn(13)
	heading := pick(r, []string{"up", "down", "left", "right"})
	dx := r.Intn(2*w-1) - (w - 1)
	dy := r.Intn(2*h-1) - (h - 1)
	if dx == 0 && dy == 0 {
		dx = 1
	}
	if r.Intn(6) == 0 { // biscuit right next to Spot
		switch r.Intn(4) {
		case 0:
			dx, dy = 1, 0
		case 1:
			dx, dy = -1, 0
		case 2:
			dx, dy = 0, 1
		default:
			dx, dy = 0, -1
		}
	}
	free := w*h - length
	var moves []string
	for _, m := range []string{"up", "down", "left", "right"} {
		if m != opposite[heading] {
			moves = append(moves, m)
		}
	}
	r.Shuffle(len(moves), func(i, j int) { moves[i], moves[j] = moves[j], moves[i] })
	moves = moves[:2+r.Intn(2)] // 2 or 3 safe moves
	// How much of the lawn the best move keeps: usually nearly all of it, sometimes Spot's
	// own body has already closed most of it off.
	frac := pick(r, []float64{0.99, 0.99, 0.99, 0.8, 0.5, 0.2})
	best := max(4, int(float64(free)*frac)-r.Intn(3))
	// Moves into the same open area reach the same room; a move into a pocket reaches less.
	room := map[string]int{}
	for _, m := range moves {
		room[m] = best
	}
	straightSafe := false
	for _, m := range moves {
		straightSafe = straightSafe || m == heading
	}
	if r.Intn(10) < 6 || !straightSafe { // keep the case informative, as the game dataset does
		trap := moves[r.Intn(len(moves))]
		room[trap] = 2 + r.Intn(max(1, best/3))
	}
	toward := func(m string) bool {
		switch m {
		case "right":
			return dx > 0
		case "left":
			return dx < 0
		case "down":
			return dy > 0
		}
		return dy < 0
	}
	eats := func(m string) bool { return abs(dx)+abs(dy) == 1 && toward(m) }
	var opts []opt
	var listed []string
	maxRoom := 0
	for _, m := range moves {
		maxRoom = max(maxRoom, room[m])
	}
	eatable := false
	for _, m := range moves {
		dir := "goes away from the biscuit"
		if eats(m) {
			dir, eatable = "eats the biscuit", true
		} else if toward(m) {
			dir = "goes toward the biscuit"
		}
		d := fmt.Sprintf("%s: %s, leaves %d open squares", m, dir, room[m])
		opts = append(opts, opt{m, d})
		listed = append(listed, d)
	}
	var winners []string
	for _, m := range moves {
		if room[m] == maxRoom {
			winners = append(winners, m)
		}
	}
	for _, m := range winners {
		if m == heading {
			winners = []string{m}
			break
		}
	}
	gw := map[string]float64{}
	for _, m := range moves {
		gw[m] = 0.08 / float64(len(moves)-len(winners)+1)
	}
	for _, m := range winners {
		gw[m] = 0.92 / float64(len(winners))
	}
	bis := []string{}
	if dx != 0 {
		bis = append(bis, fmt.Sprintf("%d %s", abs(dx), map[bool]string{true: "right", false: "left"}[dx > 0]))
	}
	if dy != 0 {
		bis = append(bis, fmt.Sprintf("%d %s", abs(dy), map[bool]string{true: "down", false: "up"}[dy > 0]))
	}
	state := fmt.Sprintf("Spot is %d squares long on a %d by %d lawn, heading %s. The biscuit is %s.", length, w, h, heading, strings.Join(bis, " and "))
	ratio := float64(maxRoom) / float64(free)
	risk := 0
	switch {
	case ratio < 0.3:
		risk = 3
	case ratio < 0.6:
		risk = 2
	case ratio < 0.95:
		risk = 1
	}
	eatP := 0.03
	if eatable {
		eatP = 0.97
	}
	safe := "Safe moves: " + strings.Join(listed, "; ") + "."
	return item(state, map[string]any{"width": w, "height": h, "length": length, "heading": heading, "dx": dx, "dy": dy,
		"free": free, "rooms": room, "winners": winners},
		map[string]schema.Question{
			"move": choiceQ("Which way should Spot step next to reach the biscuit without getting boxed in?", opts...),
			"eats_now": noulQ(safe+" One of these moves eats the biscuit right now.",
				"A safe move lands on the biscuit this step.", "No safe move reaches the biscuit this step."),
			"boxed_in_risk": scoreQ(fmt.Sprintf("%s The lawn has %d open squares. How close is Spot to being boxed in, judging by the most room any safe move leaves?", safe, free),
				"No risk: the best move keeps nearly the whole lawn open.",
				"Low: the best move keeps most of the lawn open.",
				"Elevated: even the best move leaves under 60% of the lawn.",
				"High: every safe move leaves under 30% of the lawn."),
		},
		map[string]schema.Answer{"move": dist("choice", gw), "eats_now": yes(eatP), "boxed_in_risk": level(risk, 4, 0.9)},
		"move", "eats_now", "boxed_in_risk")
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// --- it_support --------------------------------------------------------------------------

var itTickets = map[string][]string{
	"hardware": {"My laptop screen flickers and then goes black after a few minutes.",
		"The office printer on floor %d jams on every page and shows error E%d.",
		"My docking station stopped charging the laptop this morning.",
		"The badge reader at the side entrance beeps red for everyone."},
	"software": {"The expense app crashes as soon as I open the receipts tab.",
		"After last night's update the CRM shows a blank page.",
		"Excel freezes when I open the quarterly forecast file.",
		"The design tool says my licence has expired, but it was renewed."},
	"network": {"Wi-Fi in meeting room %d keeps dropping every few minutes.",
		"The VPN connects but no internal sites load.",
		"Everything on the third floor is extremely slow since lunch.",
		"I cannot reach the file server from the warehouse."},
	"account": {"I am locked out after changing my password yesterday.",
		"New starter %s has no email account yet and begins today.",
		"Please add me to the finance shared drive, my manager approved it.",
		"My multi-factor prompts stopped arriving on my new phone."},
	"security": {"I clicked a link in an email about my payslip and typed my password.",
		"There is a USB stick labelled 'salaries' left in the kitchen.",
		"Antivirus popped up a warning about a file called invoice.exe.",
		"Someone I don't know is listed as owner on our team's files."},
}

func itSupport(r *rand.Rand) schema.DecideItem {
	cat := pick(r, []string{"hardware", "software", "network", "account", "security"})
	body := pick(r, itTickets[cat])
	if strings.Contains(body, "%s") {
		body = fmt.Sprintf(body, pick(r, []string{"Ana", "Bram", "Chen", "Dee"}))
	} else if strings.Count(body, "%d") == 2 {
		body = fmt.Sprintf(body, 1+r.Intn(5), 10+r.Intn(80))
	} else if strings.Contains(body, "%d") {
		body = fmt.Sprintf(body, 1+r.Intn(9))
	}
	users := pick(r, []int{1, 1, 3, 12, 45, 300})
	critical := r.Intn(4) == 0
	vip := r.Intn(6) == 0
	workaround := r.Intn(3) == 0
	role := "staff member"
	if vip {
		role = "executive assistant to the CEO"
	}
	state := map[string]any{
		"ticket":               map[string]any{"id": fmt.Sprintf("INC-%05d", r.Intn(100000)), "from": role, "text": body},
		"users_affected":       users,
		"business_critical":    critical,
		"workaround_available": workaround,
	}
	catW := map[string]float64{"hardware": 0.025, "software": 0.025, "network": 0.025, "account": 0.025, "security": 0.025}
	catW[cat] = 0.9
	if cat == "security" && strings.Contains(body, "password") {
		catW["security"], catW["account"] = 0.75, 0.175
	}
	prio := 0
	switch {
	case users >= 300 || (critical && users >= 45) || cat == "security":
		prio = 3
	case users >= 45 || critical:
		prio = 2
	case users >= 3 || vip:
		prio = 1
	}
	if workaround && cat != "security" {
		prio = clamp(prio-1, 0, 3)
	}
	onsite := 0.05
	if cat == "hardware" {
		onsite = 0.9
		if workaround {
			onsite = 0.4
		}
	}
	return item(state, map[string]any{"category": cat, "users": users, "critical": critical, "vip": vip, "workaround": workaround},
		map[string]schema.Question{
			"category": choiceQ("What kind of problem is this ticket about?",
				opt{"hardware", "A physical device is broken or misbehaving."},
				opt{"software", "An application or update is not working."},
				opt{"network", "Connectivity: Wi-Fi, VPN, slow or unreachable systems."},
				opt{"account", "Logins, access rights, passwords or new accounts."},
				opt{"security", "A possible attack, leak or compromised credential."}),
			"priority": scoreQ("How urgently should IT pick this up?",
				"Low: next few days is fine.", "Normal: within the day.",
				"High: within the hour.", "Critical: drop everything now."),
			"needs_onsite": noulQ("A technician has to go to the person or device in person.",
				"Someone must be physically there.", "It can be handled remotely."),
		},
		map[string]schema.Answer{"category": dist("choice", catW), "priority": level(prio, 4, 0.8), "needs_onsite": yes(onsite)},
		"category", "priority", "needs_onsite")
}

// --- delivery ----------------------------------------------------------------------------

func delivery(r *rand.Rand) schema.DecideItem {
	kind := pick(r, []string{"delayed", "delayed", "address", "damaged", "lost", "failed_attempts", "on_time"})
	value := pick(r, []int{15, 40, 85, 160, 420, 900})
	days := 0
	events := []string{"Label created", "Picked up by carrier", "Arrived at sorting hub"}
	switch kind {
	case "on_time":
		events = append(events, "Out for delivery")
	case "delayed":
		days = 1 + r.Intn(7)
		events = append(events, fmt.Sprintf("Delayed at hub: %s (%d days behind schedule)", pick(r, []string{"weather", "high volume", "vehicle breakdown"}), days))
	case "address":
		events = append(events, "Delivery attempted: address incomplete, no house number", "Held at depot awaiting instructions")
	case "damaged":
		events = append(events, "Parcel damaged in transit: outer box crushed, contents visible")
	case "lost":
		days = 7 + r.Intn(10)
		events = append(events, fmt.Sprintf("No scan for %d days", days))
	case "failed_attempts":
		events = append(events, "Delivery attempt 1: recipient not home", "Delivery attempt 2: recipient not home", "Delivery attempt 3: recipient not home, returning to depot")
	}
	contacted := r.Intn(3) == 0
	state := map[string]any{"order": fmt.Sprintf("#%06d", r.Intn(1000000)), "order_value_usd": value, "tracking": events, "customer_already_contacted": contacted}
	w := map[string]float64{"wait": 0.02, "contact_customer": 0.02, "reship": 0.02, "refund": 0.02, "return_to_sender": 0.02}
	sev := 0
	notify := 0.9
	switch kind {
	case "on_time":
		w["wait"], notify = 0.95, 0.05
	case "delayed":
		if days <= 3 {
			w["wait"], w["contact_customer"], sev, notify = 0.8, 0.15, 1, 0.5
		} else {
			w["contact_customer"], w["wait"], sev = 0.65, 0.3, 2
		}
	case "address":
		w["contact_customer"], sev = 0.9, 2
	case "damaged":
		if value >= 400 {
			w["refund"], w["reship"] = 0.5, 0.45
		} else {
			w["reship"], w["refund"] = 0.75, 0.2
		}
		sev = 2
	case "lost":
		if value >= 400 {
			w["refund"], w["reship"] = 0.6, 0.35
		} else {
			w["reship"], w["refund"] = 0.7, 0.25
		}
		sev = 3
	case "failed_attempts":
		w["return_to_sender"], w["contact_customer"], sev = 0.6, 0.35, 2
	}
	if contacted && kind != "on_time" {
		notify = 0.3
	}
	if value >= 400 && sev < 3 && kind != "on_time" {
		sev++
	}
	return item(state, map[string]any{"kind": kind, "value": value, "days": days, "contacted": contacted},
		map[string]schema.Question{
			"action": choiceQ("What should the shop do next about this parcel?",
				opt{"wait", "Nothing yet: the parcel is still on its way."},
				opt{"contact_customer", "Ask or tell the customer something before going further."},
				opt{"reship", "Send a replacement parcel."},
				opt{"refund", "Give the customer their money back."},
				opt{"return_to_sender", "Let the parcel come back to the warehouse."}),
			"notify_customer": noulQ("The customer should get a message about this parcel now.",
				"Send the customer an update now.", "No message needed right now."),
			"severity": scoreQ("How bad is this for the customer?",
				"Fine: no impact.", "Minor: a small delay.", "Serious: the order needs action.", "Severe: the order is lost or worthless."),
		},
		map[string]schema.Answer{"action": dist("choice", w), "notify_customer": yes(notify), "severity": level(sev, 4, 0.8)},
		"action", "notify_customer", "severity")
}

// --- code_review -------------------------------------------------------------------------

func codeReview(r *rand.Rand) schema.DecideItem {
	area := pick(r, []string{"docs", "ui", "api", "infra", "auth", "payments"})
	lines := pick(r, []int{4, 18, 60, 140, 380, 900})
	tests := r.Intn(2) == 0
	ci := pick(r, []string{"passing", "passing", "passing", "failing", "flaky"})
	files := map[string][]string{
		"docs": {"README.md", "docs/setup.md"}, "ui": {"web/src/Button.tsx", "web/src/theme.css"},
		"api": {"server/routes/orders.go", "server/routes/users.go"}, "infra": {"deploy/terraform/main.tf", ".github/workflows/ci.yml"},
		"auth": {"server/auth/session.go", "server/auth/tokens.go"}, "payments": {"server/billing/charge.go", "server/billing/refund.go"},
	}[area]
	changed := append([]string{}, files...)
	if tests && area != "docs" {
		changed = append(changed, strings.Replace(strings.Replace(files[0], ".go", "_test.go", 1), ".tsx", ".test.tsx", 1))
	}
	state := map[string]any{
		"pull_request":  r.Intn(9000) + 1000,
		"title":         pick(r, []string{"Refactor", "Fix", "Add", "Update", "Clean up", "Speed up"}) + " " + area + " code",
		"lines_changed": lines, "files": changed, "ci": ci, "tests_added": tests && area != "docs",
	}
	w := map[string]float64{"approve": 0.03, "request_changes": 0.03, "needs_security_review": 0.03}
	sensitive := area == "auth" || area == "payments"
	switch {
	case ci == "failing":
		w["request_changes"] = 0.85
	case sensitive:
		w["needs_security_review"], w["request_changes"] = 0.75, 0.15
	case area == "docs":
		w["approve"] = 0.92
	case lines > 300 && !tests:
		w["request_changes"], w["approve"] = 0.7, 0.25
	case tests && ci == "passing":
		w["approve"] = 0.85
	case ci == "flaky":
		w["request_changes"], w["approve"] = 0.5, 0.45
	default:
		w["approve"], w["request_changes"] = 0.55, 0.4
	}
	risk := map[string]int{"docs": 0, "ui": 1, "api": 1, "infra": 2, "auth": 3, "payments": 3}[area]
	if lines > 300 {
		risk++
	}
	if tests {
		risk--
	}
	risk = clamp(risk, 0, 3)
	needTests := 0.1
	if !tests && area != "docs" && lines > 20 {
		needTests = 0.9
	}
	return item(state, map[string]any{"area": area, "lines": lines, "tests": tests, "ci": ci},
		map[string]schema.Question{
			"decision": choiceQ("What should the reviewer do with this pull request?",
				opt{"approve", "It can be merged as it is."},
				opt{"request_changes", "The author has to fix something first."},
				opt{"needs_security_review", "A security reviewer must look at it before merging."}),
			"risk": scoreQ("How risky is merging this change?",
				"Trivial.", "Low.", "Moderate: could break something users notice.", "High: could leak data or money."),
			"needs_tests": noulQ("The pull request needs more tests before it can be merged.",
				"Tests are missing for this change.", "The test coverage is fine or not needed."),
		},
		map[string]schema.Answer{"decision": dist("choice", w), "risk": level(risk, 4, 0.8), "needs_tests": yes(needTests)},
		"decision", "risk", "needs_tests")
}

// --- calendar ----------------------------------------------------------------------------

type meeting struct {
	Title    string `json:"title"`
	Start    string `json:"start"`
	Minutes  int    `json:"minutes"`
	Priority string `json:"priority"`
	Movable  bool   `json:"can_be_moved"`
	start    int
}

func calendar(r *rand.Rand) schema.DecideItem {
	titles := []string{"Team stand-up", "Client call", "Dentist", "1:1 with manager", "Budget review", "School pick-up", "Interview", "Design critique"}
	r.Shuffle(len(titles), func(i, j int) { titles[i], titles[j] = titles[j], titles[i] })
	mk := func(t string) meeting {
		s := 8*60 + 15*r.Intn(36)
		return meeting{Title: t, Start: fmt.Sprintf("%02d:%02d", s/60, s%60), Minutes: pick(r, []int{15, 30, 45, 60, 90}),
			Priority: pick(r, []string{"low", "normal", "high"}), Movable: r.Intn(2) == 0, start: s}
	}
	a, b := mk(titles[0]), mk(titles[1])
	if r.Intn(2) == 0 { // force a clash half of the time
		b.start = a.start + 15*r.Intn(max(1, a.Minutes/15))
		b.Start = fmt.Sprintf("%02d:%02d", b.start/60, b.start%60)
	}
	ms := []meeting{a, b}
	sort.Slice(ms, func(i, j int) bool { return ms[i].start < ms[j].start })
	a, b = ms[0], ms[1]
	clash := b.start < a.start+a.Minutes
	hours := pick(r, []int{1, 5, 20, 50, 120})
	state := map[string]any{"meetings_same_day": []meeting{a, b}, "day_starts_in_hours": hours}
	rank := map[string]int{"low": 0, "normal": 1, "high": 2}
	w := map[string]float64{"keep_both": 0.02, "move_first": 0.02, "move_second": 0.02, "decline_second": 0.02}
	switch {
	case !clash:
		w["keep_both"] = 0.95
	case b.Movable && (!a.Movable || rank[b.Priority] <= rank[a.Priority]):
		w["move_second"], w["move_first"] = 0.75, 0.15
	case a.Movable:
		w["move_first"], w["move_second"] = 0.75, 0.15
	case rank[b.Priority] < rank[a.Priority]:
		w["decline_second"] = 0.85
	default:
		w["decline_second"], w["move_first"] = 0.5, 0.4
	}
	urg := 0
	switch {
	case hours < 2:
		urg = 3
	case hours < 24:
		urg = 2
	case hours < 72:
		urg = 1
	}
	if !clash {
		urg = 0
	}
	cp := 0.02
	if clash {
		cp = 0.98
	}
	return item(state, map[string]any{"clash": clash, "hours": hours},
		map[string]schema.Question{
			"clash": noulQ("The two meetings overlap in time.", "They overlap.", "They do not overlap."),
			"resolution": choiceQ("What is the best way to handle these two meetings?",
				opt{"keep_both", "Nothing to change."},
				opt{"move_first", "Reschedule the earlier meeting."},
				opt{"move_second", "Reschedule the later meeting."},
				opt{"decline_second", "Decline the later meeting."}),
			"urgency": scoreQ("How soon does someone have to act on the calendar?",
				"No action needed.", "Within a few days.", "Today.", "Right now."),
		},
		map[string]schema.Answer{"clash": yes(cp), "resolution": dist("choice", w), "urgency": level(urg, 4, 0.85)},
		"clash", "resolution", "urgency")
}
