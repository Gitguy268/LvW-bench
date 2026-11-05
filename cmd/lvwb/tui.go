package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Gitguy268/LvW-bench/internal/presets"
	"github.com/Gitguy268/LvW-bench/internal/results"
	"github.com/Gitguy268/LvW-bench/internal/runner"
	"github.com/Gitguy268/LvW-bench/internal/schema"
)

var (
	stTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0B04A"))
	stDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	stSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(lipgloss.Color("#E0B04A"))
	stOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FB760"))
	stErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C5A"))
	stBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

type evMsg runner.Event
type doneMsg struct {
	res  *results.Result
	path string
	err  error
}

type trackState struct {
	done, total, correct int
	config               string
	final                string
}

type tuiModel struct {
	opts     *runner.Options
	ctx      context.Context
	cancel   context.CancelFunc
	events   chan tea.Msg
	picking  bool
	input    textinput.Model
	cursor   int
	profiles []string
	profile  int
	running  bool
	log      []string
	download string
	tracks   map[string]*trackState
	res      *results.Result
	path     string
	err      error
	started  time.Time
	width    int
}

func runTUI(ctx context.Context, o *runner.Options) error {
	ti := textinput.New()
	ti.Placeholder = "HF link or id, path to .gguf / safetensors folder, or server URL"
	ti.CharLimit = 400
	ti.Width = 70
	m := &tuiModel{opts: o, events: make(chan tea.Msg, 256), input: ti, tracks: map[string]*trackState{},
		profiles: []string{"quick", "standard", "full"}, profile: 1}
	for i, p := range m.profiles {
		if p == o.Profile {
			m.profile = i
		}
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	defer m.cancel()
	if o.Model == "" {
		m.picking = true
		m.input.Focus()
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil && err != tea.ErrProgramKilled {
		return err
	}
	if fm, ok := final.(*tuiModel); ok {
		if fm.res != nil {
			printResult(fm.res)
			fmt.Println("\nsaved", fm.path)
		}
		return fm.err
	}
	return nil
}

func (m *tuiModel) Init() tea.Cmd {
	if !m.picking {
		return m.start()
	}
	return textinput.Blink
}

func (m *tuiModel) start() tea.Cmd {
	m.running, m.picking, m.started = true, false, time.Now()
	m.opts.Profile = m.profiles[m.profile]
	o := *m.opts
	o.Events = func(e runner.Event) {
		select {
		case m.events <- evMsg(e):
		default: // drop progress if the UI is behind
		}
	}
	go func() {
		res, preds, err := runner.Run(m.ctx, o)
		path := ""
		if err == nil {
			path, err = results.Save(os.Getenv("LVWB_OUT"), res, preds)
		}
		m.events <- doneMsg{res, path, err}
	}()
	return m.wait()
}

func (m *tuiModel) wait() tea.Cmd { return func() tea.Msg { return <-m.events } }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancel()
			if m.running {
				m.err = context.Canceled
			}
			return m, tea.Quit
		case "q":
			if !m.picking {
				m.cancel()
				return m, tea.Quit
			}
		}
		if m.picking {
			switch msg.String() {
			case "up":
				m.cursor = max(0, m.cursor-1)
				return m, nil
			case "down":
				m.cursor = min(len(presets.All)-1, m.cursor+1)
				return m, nil
			case "tab":
				m.profile = (m.profile + 1) % len(m.profiles)
				return m, nil
			case "enter":
				if v := strings.TrimSpace(m.input.Value()); v != "" {
					m.opts.Model = v
				} else {
					p := presets.All[m.cursor]
					m.opts.Model = p.Input
					if len(m.opts.Tracks) == 0 {
						m.opts.Tracks = p.Tracks
					}
					if p.Mode != "" && m.opts.Mode == "general" {
						m.opts.Mode = p.Mode
					}
					if len(p.Args) > 0 {
						if extra, _, _, err := runFlags(p.Args); err == nil {
							m.opts.Resolve.Subfolder = extra.Resolve.Subfolder
							m.opts.Configs = extra.Configs
						}
					}
				}
				return m, m.start()
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	case evMsg:
		e := runner.Event(msg)
		switch e.Kind {
		case "status":
			s := e.Msg
			if e.Track != "" {
				s = e.Track + ": " + s
			}
			m.log = append(m.log, s)
			m.download = ""
		case "download":
			pct := 0.0
			if e.OfBytes > 0 {
				pct = float64(e.Bytes) / float64(e.OfBytes)
			}
			m.download = fmt.Sprintf("↓ %s %s %.2f / %.2f GB", e.Msg, bar(pct, 24), float64(e.Bytes)/1e9, float64(e.OfBytes)/1e9)
		case "progress":
			t := m.track(e.Track)
			t.done, t.total, t.correct, t.config = e.Done, e.Total, e.Correct, e.Config
		case "track_done":
			m.track(e.Track).final = e.Msg
		}
		return m, m.wait()
	case doneMsg:
		m.running = false
		m.res, m.path, m.err = msg.res, msg.path, msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m *tuiModel) track(name string) *trackState {
	if m.tracks[name] == nil {
		m.tracks[name] = &trackState{}
	}
	return m.tracks[name]
}

func bar(f float64, w int) string {
	n := int(f*float64(w) + 0.5)
	n = max(0, min(w, n))
	return stOK.Render(strings.Repeat("█", n)) + stDim.Render(strings.Repeat("░", w-n))
}

func (m *tuiModel) View() string {
	var b strings.Builder
	b.WriteString(stTitle.Render("LvW-bench") + stDim.Render("  tiny models, System 1 + System 2, fully local") + "\n\n")
	if m.picking {
		b.WriteString("Model: " + m.input.View() + "\n\n")
		b.WriteString(stDim.Render("or pick a preset (↑/↓, enter):") + "\n")
		for i, p := range presets.All {
			line := fmt.Sprintf(" %-24s %-10s %s ", p.Name, p.Kind, p.Comment)
			if i == m.cursor {
				line = stSel.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + stDim.Render("profile: ") + m.profiles[m.profile] + stDim.Render("   tab: change profile · esc: quit") + "\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("%s  %s\n\n", stTitle.Render(m.opts.Model), stDim.Render("profile "+m.opts.Profile)))
	start := max(0, len(m.log)-6)
	for _, l := range m.log[start:] {
		b.WriteString(stDim.Render("· ") + l + "\n")
	}
	if m.download != "" {
		b.WriteString(m.download + "\n")
	}
	b.WriteString("\n")
	var rows []string
	for _, tr := range schema.AllTracks {
		t := m.tracks[tr]
		if t == nil {
			continue
		}
		f := float64(t.done) / float64(max(1, t.total))
		acc := 100 * float64(t.correct) / float64(max(1, t.done))
		row := fmt.Sprintf("%-7s %s %4d/%-4d  %5.1f%% correct", tr, bar(f, 30), t.done, t.total, acc)
		if t.final != "" {
			row += "  " + stOK.Render("✓ "+t.final)
		} else if t.config != "" {
			row += stDim.Render("  " + t.config)
		}
		rows = append(rows, row)
	}
	if len(rows) > 0 {
		b.WriteString(stBox.Render(strings.Join(rows, "\n")) + "\n")
	}
	if m.err != nil && m.err != context.Canceled {
		b.WriteString(stErr.Render("error: "+m.err.Error()) + "\n")
	}
	b.WriteString("\n" + stDim.Render(fmt.Sprintf("%s elapsed · q: stop", time.Since(m.started).Round(time.Second))) + "\n")
	return b.String()
}
