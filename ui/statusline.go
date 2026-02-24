package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type StatusLine struct {
	width int

	hasStatus bool
	kind      StatusKind
	text      string

	promptActive bool
	promptKind   PromptKind
	promptLabel  string
	input        textinput.Model
}

func NewStatusLine() StatusLine {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256
	ti.Width = 40
	ti.Focus()

	return StatusLine{width: 80, input: ti}
}

func (s StatusLine) SetSize(width int) StatusLine {
	s.width = width
	s.input.Width = Max(10, width-45)
	return s
}

func (s StatusLine) SetStatus(k StatusKind, txt string) StatusLine {
	s.hasStatus = true
	s.kind = k
	s.text = txt
	return s
}

func (s StatusLine) BeginPrompt(kind PromptKind, label, placeholder, initial string) StatusLine {
	s.promptActive = true
	s.promptKind = kind
	s.promptLabel = label
	s.input.Placeholder = placeholder
	s.input.SetValue(initial)
	s.input.CursorEnd()
	s.input.Focus()
	return s
}

func (s StatusLine) EndPrompt() StatusLine {
	s.promptActive = false
	s.input.Blur()
	return s
}

func (s StatusLine) IsPromptActive() bool { return s.promptActive }

func (s StatusLine) Update(msg tea.KeyMsg) (StatusLine, tea.Cmd) {
	if !s.promptActive {
		return s, nil
	}
	switch msg.String() {
	case "esc":
		return s, func() tea.Msg { return PromptResultMsg{Kind: s.promptKind, Accepted: false, Value: ""} }
	case "enter":
		val := strings.TrimSpace(s.input.Value())
		return s, func() tea.Msg { return PromptResultMsg{Kind: s.promptKind, Accepted: true, Value: val} }
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, cmd
}

func (s StatusLine) View() string {
	style := lipgloss.NewStyle().
		Width(s.width-2).
		Height(1).
		Background(T.BG).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(T.Border).
		Padding(0, 1)

	if s.promptActive {
		lbl := Warn.Render(s.promptLabel) + Dim.Render(": ")
		return style.Render(lbl + s.input.View() + Dim.Render("  (Enter=OK · Esc=Cancel)"))
	}

	txt := "Ready"
	if s.hasStatus && strings.TrimSpace(s.text) != "" {
		txt = s.text
	}

	switch s.kind {
	case StatusWarn:
		return style.Render(Warn.Render(txt))
	case StatusError:
		return style.Render(Error.Render(txt))
	default:
		return style.Render(Base.Render(txt))
	}
}
