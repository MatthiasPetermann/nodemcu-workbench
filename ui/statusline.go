package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type StatusLine struct {
	width int

	hasStatus bool
	kind      StatusKind
	text      string

	progressActive bool
	progressPhase  string
	progressDone   int
	progressTotal  int

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

func (s StatusLine) SetProgress(active bool, phase string, done, total int) StatusLine {
	s.progressActive = active
	s.progressPhase = phase
	s.progressDone = done
	s.progressTotal = total
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
	if s.promptKind == PromptConfirmDelete {
		switch msg.String() {
		case "esc":
			return s, func() tea.Msg { return PromptResultMsg{Kind: s.promptKind, Accepted: false, Value: ""} }
		case "enter":
			return s, func() tea.Msg { return PromptResultMsg{Kind: s.promptKind, Accepted: true, Value: ""} }
		default:
			return s, nil
		}
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
		if s.promptKind == PromptConfirmDelete {
			msg := clampRunes(s.promptLabel+" · Enter=Bestätigen · Esc=Abbrechen", Max(1, s.width-8))
			return style.Render(Warn.Render(msg))
		}
		lbl := Warn.Render(clampRunes(s.promptLabel, Max(1, s.width-24))) + Dim.Render(": ")
		return style.Render(lbl + s.input.View() + Dim.Render("  (Enter=OK · Esc=Cancel)"))
	}

	if s.progressActive {
		inner := Max(10, s.width-8)
		phase := clampRunes("FLASH "+s.progressPhase, inner/3)
		total := s.progressTotal
		if total <= 0 {
			total = 1
		}
		done := s.progressDone
		if done < 0 {
			done = 0
		}
		if done > total {
			done = total
		}
		barW := Max(8, inner-len(phase)-20)
		fill := int(float64(done) / float64(total) * float64(barW))
		if fill > barW {
			fill = barW
		}
		bar := strings.Repeat("█", fill) + strings.Repeat("░", barW-fill)
		pct := int(float64(done) / float64(total) * 100)
		line := clampRunes(phase+" ["+bar+"] "+fmt.Sprintf("%3d%% %d/%d", pct, done, total), inner)
		return style.Render(Accent.Render(line))
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

func clampRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
