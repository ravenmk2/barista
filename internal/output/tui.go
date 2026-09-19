package output

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type startMsg struct{ name string }
type resultMsg struct{ res Result }
type finishMsg struct{}

type model struct {
	command  string
	names    []string
	color    bool
	started  chan struct{}
	running  []string
	ok       int
	skipped  int
	failed   int
	height   int
	finished bool
}

func (m model) Init() tea.Cmd {
	close(m.started)
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.finished = true
			return m, tea.Quit
		}
	case startMsg:
		m.running = append(m.running, msg.name)
		return m, nil
	case resultMsg:
		for i, n := range m.running {
			if n == msg.res.Name {
				m.running = append(m.running[:i], m.running[i+1:]...)
				break
			}
		}
		switch msg.res.Status {
		case StatusOK:
			m.ok++
		case StatusSkipped:
			m.skipped++
		case StatusFailed:
			m.failed++
		}
		return m, nil
	case finishMsg:
		m.finished = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) progressLine(lp lipPalette) string {
	done := m.ok + m.skipped + m.failed
	seg := func(v int, label string, style func(string) string) string {
		t := fmt.Sprintf("%d %s", v, label)
		if v > 0 {
			return style(t)
		}
		return lp.Dim(t)
	}
	return lp.Dim(fmt.Sprintf("%d/%d done", done, len(m.names))) + "  " +
		seg(m.ok, "ok", lp.Green) + lp.Dim(", ") +
		seg(m.skipped, "skipped", lp.Yellow) + lp.Dim(", ") +
		seg(m.failed, "failed", lp.Red)
}

func (m model) View() string {
	if m.finished {
		return ""
	}
	lp := lipPalette{enabled: m.color}
	var b strings.Builder
	fmt.Fprintf(&b, "barista %s\n", m.command)
	b.WriteString(m.progressLine(lp))
	limit := len(m.running)
	if m.height > 0 && limit > m.height-3 {
		limit = max(1, m.height-3)
	}
	for _, name := range m.running[:limit] {
		fmt.Fprintf(&b, "\n  %s %s %s", m.icon(""), name, lp.Dim("running"))
	}
	if hidden := len(m.running) - limit; hidden > 0 {
		fmt.Fprintf(&b, "\n  %s", lp.Dim(fmt.Sprintf("+ %d more", hidden)))
	}
	return b.String()
}

var (
	styleOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleSkipped    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleFailed     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePending    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleCyan       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleBlue       = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleMagenta    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleYellowBold = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
)

type lipPalette struct{ enabled bool }

func (p lipPalette) render(st lipgloss.Style, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return st.Render(s)
}

func (p lipPalette) Green(s string) string      { return p.render(styleOK, s) }
func (p lipPalette) Red(s string) string        { return p.render(styleFailed, s) }
func (p lipPalette) Yellow(s string) string     { return p.render(styleSkipped, s) }
func (p lipPalette) Cyan(s string) string       { return p.render(styleCyan, s) }
func (p lipPalette) Blue(s string) string       { return p.render(styleBlue, s) }
func (p lipPalette) Magenta(s string) string    { return p.render(styleMagenta, s) }
func (p lipPalette) Dim(s string) string        { return p.render(styleDim, s) }
func (p lipPalette) YellowBold(s string) string { return p.render(styleYellowBold, s) }

func (p lipPalette) Branch(s string) string {
	switch branchClass(s) {
	case classTrunk:
		return p.Green(s)
	case classDev:
		return p.Blue(s)
	case classFeature:
		return p.Magenta(s)
	case classRelease:
		return p.Cyan(s)
	case classFix:
		return p.Yellow(s)
	}
	return s
}

func symbol(s Status) string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusSkipped:
		return "~"
	case StatusFailed:
		return "✗"
	}
	return "…"
}

func (m model) icon(s Status) string {
	if !m.color {
		switch s {
		case StatusOK:
			return "[ok]"
		case StatusSkipped:
			return "[skipped]"
		case StatusFailed:
			return "[failed]"
		}
		return "[..]"
	}
	switch s {
	case StatusOK:
		return styleOK.Render(symbol(s))
	case StatusSkipped:
		return styleSkipped.Render(symbol(s))
	case StatusFailed:
		return styleFailed.Render(symbol(s))
	}
	return stylePending.Render(symbol(s))
}

type Panel struct {
	prog      *tea.Program
	m         model
	mu        sync.Mutex
	alive     bool
	nameWidth int
}

func NewPanel(command string, names []string, color bool) *Panel {
	m := model{
		command: command,
		names:   names,
		color:   color,
		started: make(chan struct{}),
	}
	return &Panel{prog: tea.NewProgram(m), m: m, alive: true, nameWidth: NameWidth(names)}
}

func (p *Panel) Started() <-chan struct{} { return p.m.started }

func (p *Panel) Start(name string) { p.prog.Send(startMsg{name}) }

func (p *Panel) OnResult(_ int, res Result) {
	p.prog.Send(resultMsg{res})
	var buf bytes.Buffer
	NewTextRenderer(&buf, p.m.command, p.m.color, p.nameWidth).OnResult(0, res)
	p.mu.Lock()
	alive := p.alive
	p.mu.Unlock()
	if alive {
		p.prog.Println(strings.TrimRight(buf.String(), "\n"))
	}
}

func (p *Panel) Finish() { p.prog.Send(finishMsg{}) }

func (p *Panel) Wait() error {
	_, err := p.prog.Run()
	p.mu.Lock()
	p.alive = false
	p.mu.Unlock()
	return err
}
