package terminal

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"nodemcu-workbench/repl"
	"nodemcu-workbench/ui"
)

/* =========================
   MODEL
========================= */

type Model struct {
	w, h int

	session   *repl.Session
	connected bool

	vp  viewport.Model
	in  textinput.Model
	log []string

	// REPL state
	pending []string // multiline buffer
	cont    bool     // continuation active?
}

/* =========================
   PROMPT
========================= */

func prompt(cont bool) string {
	if cont {
		return ui.Accent.Render("»» ")
	}
	return ui.Accent.Render("» ")
}

/* =========================
   INIT
========================= */

func New(sess *repl.Session) Model {
	in := textinput.New()
	in.Prompt = prompt(false)
	in.CharLimit = 512
	in.Focus()

	vp := viewport.New(0, 0)

	m := Model{
		session:   sess,
		connected: sess != nil,
		vp:        vp,
		in:        in,
		log:       nil,
	}

	if sess != nil {
		m.log = append(m.log, ui.Ok.Render("connected"))
	} else {
		m.log = append(m.log, ui.Warn.Render("not connected"))
	}

	return m
}

/* =========================
   LAYOUT
========================= */

func (m Model) SetSize(w, h int) Model {
	m.w, m.h = w, h

	innerW := ui.Max(10, w-6)
	innerH := ui.Max(4, h-4)

	m.vp.Width = innerW
	m.vp.Height = ui.Max(1, innerH-2)
	m.in.Width = ui.Max(10, innerW-2)

	m.refresh()
	return m
}

/* =========================
   TEA
========================= */

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) SetSession(sess *repl.Session) Model {
	m.session = sess
	m.connected = sess != nil
	if sess == nil {
		m.append(ui.Warn.Render("serial released (maintenance mode)"))
	} else {
		m.append(ui.Ok.Render("serial connected"))
	}
	m.refresh()
	return m
}

/*
Internal messages
*/
type replContinueMsg struct{}
type replDoneMsg struct {
	out []string
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case replContinueMsg:
		m.cont = true
		m.in.Prompt = prompt(true)
		return m, nil

	case replDoneMsg:
		for _, l := range msg.out {
			m.append(l)
		}
		m.pending = nil
		m.cont = false
		m.in.Prompt = prompt(false)
		m.refresh()
		return m, nil
	}

	return m, nil
}

/* =========================
   INPUT
========================= */

func (m Model) UpdateKeys(k tea.KeyMsg) (Model, tea.Cmd, bool) {

	switch k.String() {

	case "f2":
		m.log = nil
		m.refresh()
		return m, statusInfo("Screen cleared"), true

	case "f5":
		return m.reconnect(), nil, true

	case "ctrl+c":
		if m.cont {
			_ = m.session.Interrupt()
			m.pending = nil
			m.cont = false
			m.in.Prompt = prompt(false)
			m.append(ui.Warn.Render("interrupted"))
			m.refresh()
			return m, nil, true
		}

	case "enter":
		line := strings.TrimRight(m.in.Value(), " \t")
		m.in.SetValue("")

		if line == "" && !m.cont {
			return m, nil, true
		}

		m.append(ui.Dim.Render(m.in.Prompt) + line)

		if !m.connected || m.session == nil {
			m.append(ui.Warn.Render("not connected"))
			m.refresh()
			return m, nil, true
		}

		m.pending = append(m.pending, line)
		return m, m.execPending(), true

	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(k)
		return m, cmd, true

	default:
		var cmd tea.Cmd
		m.in, cmd = m.in.Update(k)
		return m, cmd, true
	}

	return m, nil, false
}

/* =========================
   COMMANDS
========================= */

func (m Model) execPending() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()

		//block := strings.Join(m.pending, "\n")

		//out, err := m.session.Exec(ctx, block)
		out, err := m.session.Exec(ctx, m.pending[len(m.pending)-1])
		if err == repl.ErrIncompleteInput {
			return replContinueMsg{}
		}
		if err != nil {
			return ui.StatusMsg{Kind: ui.StatusError, Text: err.Error()}
		}

		return replDoneMsg{out: out}
	}
}

func (m Model) reconnect() Model {
	if m.session == nil {
		m.append(ui.Warn.Render("no session"))
		m.refresh()
		return m
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := m.session.Sync(ctx); err != nil {
		m.connected = false
		m.append(ui.Error.Render("reconnect failed: " + err.Error()))
	} else {
		m.connected = true
		m.append(ui.Ok.Render("reconnected"))
	}

	m.refresh()
	return m
}

/* =========================
   VIEW
========================= */

func (m Model) View() string {
	w := ui.Max(1, m.w)
	h := ui.Max(1, m.h)

	title := ui.Accent.Render("Terminal")

	sep := ui.Rule.Render(strings.Repeat("─", ui.Max(0, w-6)))

	body := title +
		"\n" + sep +
		"\n" + m.vp.View() +
		"\n" + sep +
		"\n" + m.in.View()

	box := ui.Frame.
		Width(w-2).
		Height(h).
		Padding(0, 1)

	return box.Render(body)
}

/* =========================
   HELPERS
========================= */

func (m *Model) append(s string) {
	m.log = append(m.log, s)
}

func (m *Model) refresh() {
	m.vp.SetContent(strings.Join(m.log, "\n"))
	m.vp.GotoBottom()
}

func statusInfo(s string) tea.Cmd {
	return func() tea.Msg {
		return ui.StatusMsg{Kind: ui.StatusInfo, Text: s}
	}
}
