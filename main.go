package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nodemcu-workbench/modes/maintenance"
	"nodemcu-workbench/modes/terminal"
	"nodemcu-workbench/modes/workbench"
	"nodemcu-workbench/repl"
	"nodemcu-workbench/ui"
)

type appModel struct {
	w, h int

	tool string
	ctx  string
	mode ui.Mode

	port string
	baud int
	sess *repl.Session

	status ui.StatusLine

	wb workbench.Model
	tt terminal.Model
	mm maintenance.Model
}

func newApp() appModel {
	port := os.Getenv("NODEMCU_PORT")
	if port == "" {
		port = "/dev/ttyUSB0"
	}
	baud := 115200

	var sess *repl.Session
	if s, err := repl.Open(port, baud); err == nil {
		_ = s.Sync(context.Background())
		sess = s
	}

	return appModel{
		tool:   "nodemcu-workbench",
		ctx:    port,
		mode:   ui.ModeWorkbench,
		port:   port,
		baud:   baud,
		sess:   sess,
		status: ui.NewStatusLine(),
		wb:     workbench.New(sess),
		tt:     terminal.New(sess),
		mm:     maintenance.New(),
	}
}

func (m appModel) withSession(sess *repl.Session) appModel {
	m.sess = sess
	m.wb = m.wb.SetSession(sess)
	m.tt = m.tt.SetSession(sess)
	return m
}

func (m appModel) ensureSession() (appModel, error) {
	if m.sess != nil {
		return m, nil
	}
	s, err := repl.Open(m.port, m.baud)
	if err != nil {
		return m, err
	}
	_ = s.Sync(context.Background())
	return m.withSession(s), nil
}

func (m appModel) releaseSession() appModel {
	if m.sess != nil {
		_ = m.sess.Close()
	}
	return m.withSession(nil)
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(m.wb.Init(), m.tt.Init(), m.mm.Init())
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.status = m.status.SetSize(m.w)
		cw, ch := m.contentSize()
		m.wb = m.wb.SetSize(cw, ch)
		m.tt = m.tt.SetSize(cw, ch)
		m.mm = m.mm.SetSize(cw, ch)
		return m, nil

	case ui.StatusMsg:
		m.status = m.status.SetStatus(msg.Kind, msg.Text)
		return m, nil

	case ui.PromptResultMsg:
		m.status = m.status.EndPrompt()
		switch m.mode {
		case ui.ModeWorkbench:
			var cmd tea.Cmd
			m.wb, cmd = m.wb.OnPrompt(msg)
			return m, cmd
		case ui.ModeMaintenance:
			var cmd tea.Cmd
			m.mm, cmd = m.mm.OnPrompt(msg)
			return m, cmd
		default:
			return m, nil
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "f10" {
			m = m.releaseSession()
			return m, tea.Quit
		}

		if m.status.IsPromptActive() {
			var cmd tea.Cmd
			m.status, cmd = m.status.Update(msg)
			return m, cmd
		}

		if msg.String() == "f9" {
			newMode := (m.mode + 1) % 3
			m.mode = newMode
			if newMode == ui.ModeMaintenance {
				m = m.releaseSession()
				m.status = m.status.SetStatus(ui.StatusInfo, "Switched to "+m.mode.String()+" (serial released)")
				return m, nil
			}

			var err error
			m, err = m.ensureSession()
			if err != nil {
				m.status = m.status.SetStatus(ui.StatusWarn, "Switched to "+m.mode.String()+" (serial open failed: "+err.Error()+")")
				return m, nil
			}
			m.status = m.status.SetStatus(ui.StatusInfo, "Switched to "+m.mode.String())
			return m, nil
		}

		switch m.mode {
		case ui.ModeWorkbench:
			var cmd tea.Cmd
			var pr ui.PromptRequest
			var handled bool
			m.wb, cmd, pr, handled = m.wb.UpdateKeys(msg)
			if pr.Active {
				m.status = m.status.BeginPrompt(pr.Kind, pr.Label, pr.Placeholder, pr.Initial)
				return m, nil
			}
			if handled {
				return m, cmd
			}
			return m, nil

		case ui.ModeTerminal:
			var cmd tea.Cmd
			var handled bool
			m.tt, cmd, handled = m.tt.UpdateKeys(msg)
			if handled {
				return m, cmd
			}
			return m, nil

		case ui.ModeMaintenance:
			var cmd tea.Cmd
			var pr ui.PromptRequest
			var handled bool
			m.mm, cmd, pr, handled = m.mm.UpdateKeys(msg)
			if pr.Active {
				m.status = m.status.BeginPrompt(pr.Kind, pr.Label, pr.Placeholder, pr.Initial)
				return m, nil
			}
			if handled {
				return m, cmd
			}
			return m, nil
		}
	}

	switch m.mode {
	case ui.ModeWorkbench:
		var cmd tea.Cmd
		m.wb, cmd = m.wb.Update(msg)
		return m, cmd
	case ui.ModeTerminal:
		var cmd tea.Cmd
		m.tt, cmd = m.tt.Update(msg)
		return m, cmd
	case ui.ModeMaintenance:
		var cmd tea.Cmd
		m.mm, cmd = m.mm.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m appModel) View() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}

	header := ui.Header(m.w, m.tool, m.mode, m.ctx)

	var content string
	switch m.mode {
	case ui.ModeWorkbench:
		content = m.wb.View()
	case ui.ModeTerminal:
		content = m.tt.View()
	case ui.ModeMaintenance:
		content = m.mm.View()
	default:
		content = ""
	}

	cw, ch := m.contentSize()
	contentBox := lipgloss.NewStyle().
		Width(cw).
		Height(ch).
		Background(ui.T.BG).
		Render(content)

	body := ui.JoinVertical(
		header,
		contentBox,
		m.status.View(),
		ui.KeyBar(m.w, m.keys()),
	)

	return lipgloss.NewStyle().
		Width(m.w).
		Height(m.h).
		Background(ui.T.BG).
		Render(body)
}

func (m appModel) keys() []ui.FKey {
	switch m.mode {
	case ui.ModeWorkbench:
		return []ui.FKey{{Key: "F2", Label: "Refresh"}, {Key: "F4", Label: "Edit"}, {Key: "F5", Label: "Copy"}, {Key: "F6", Label: "Rename"}, {Key: "F7", Label: "New Dir"}, {Key: "F8", Label: "Delete"}, {Key: "F9", Label: "Mode"}, {Key: "F10", Label: "Quit"}}
	case ui.ModeTerminal:
		return []ui.FKey{{Key: "F2", Label: "Clear"}, {Key: "F5", Label: "Reconnect"}, {Key: "F9", Label: "Mode"}, {Key: "F10", Label: "Quit"}}
	case ui.ModeMaintenance:
		return []ui.FKey{{Key: "F5", Label: "Select"}, {Key: "F8", Label: "Flash"}, {Key: "F9", Label: "Mode"}, {Key: "F10", Label: "Quit"}}
	default:
		return []ui.FKey{{Key: "F9", Label: "Mode"}, {Key: "F10", Label: "Quit"}}
	}
}

func (m appModel) contentSize() (int, int) {
	ch := m.h - 6
	if ch < 1 {
		ch = 1
	}
	return m.w, ch
}

func main() {
	app := newApp()
	defer func() {
		if app.sess != nil {
			_ = app.sess.Close()
		}
	}()

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
