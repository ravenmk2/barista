package output

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type startMsg struct{ name string }
type resultMsg struct{ res Result }
type finishMsg struct{}

type model struct {
	command  string
	names    []string
	palette  Palette
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

func (m model) progressLine(p Palette) string {
	done := m.ok + m.skipped + m.failed
	seg := func(v int, label string, style func(string) string) string {
		t := fmt.Sprintf("%d %s", v, label)
		if v > 0 {
			return style(t)
		}
		return p.Dim(t)
	}
	return p.Dim(fmt.Sprintf("%d/%d done", done, len(m.names))) + "  " +
		seg(m.ok, "ok", p.Green) + p.Dim(", ") +
		seg(m.skipped, "skipped", p.Yellow) + p.Dim(", ") +
		seg(m.failed, "failed", p.Red)
}

func (m model) View() string {
	if m.finished {
		return ""
	}
	p := m.palette
	var b strings.Builder
	fmt.Fprintf(&b, "barista %s\n", m.command)
	b.WriteString(m.progressLine(p))
	limit := len(m.running)
	if m.height > 0 && limit > m.height-3 {
		limit = max(1, m.height-3)
	}
	for _, name := range m.running[:limit] {
		fmt.Fprintf(&b, "\n  %s %s %s", m.icon(""), name, p.Dim("running"))
	}
	if hidden := len(m.running) - limit; hidden > 0 {
		fmt.Fprintf(&b, "\n  %s", p.Dim(fmt.Sprintf("+ %d more", hidden)))
	}
	return b.String()
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
	if !m.palette.Enabled() {
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
		return m.palette.Green(symbol(s))
	case StatusSkipped:
		return m.palette.Yellow(symbol(s))
	case StatusFailed:
		return m.palette.Red(symbol(s))
	}
	return m.palette.Dim(symbol(s))
}

type Panel struct {
	prog      *tea.Program
	m         model
	mu        sync.Mutex
	alive     bool
	nameWidth int
}

func NewPanel(command string, names []string, palette Palette) *Panel {
	m := model{
		command: command,
		names:   names,
		palette: palette,
		started: make(chan struct{}),
	}
	return &Panel{prog: tea.NewProgram(m), m: m, alive: true, nameWidth: NameWidth(names)}
}

func (p *Panel) Started() <-chan struct{} { return p.m.started }

func (p *Panel) Start(name string) { p.prog.Send(startMsg{name}) }

func (p *Panel) OnResult(_ int, res Result) {
	p.prog.Send(resultMsg{res})
	var buf bytes.Buffer
	NewTextRenderer(&buf, p.m.command, p.m.palette, p.nameWidth).OnResult(0, res)
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
