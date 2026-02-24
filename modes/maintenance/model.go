package maintenance

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"nodemcu-workbench/ui"
)

/* =========================
   LIST ITEM
========================= */

type actionItem string

func (i actionItem) Title() string       { return string(i) }
func (i actionItem) Description() string { return "" }
func (i actionItem) FilterValue() string { return string(i) }

/* =========================
   MODEL
========================= */

type Model struct {
	w, h int

	actions list.Model

	pendingAction string
}

/* =========================
   INIT
========================= */

func New() Model {
	items := []list.Item{
		actionItem("Identify Device"),
		actionItem("Erase Flash"),
		actionItem("Flash Firmware"),
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Maintenance"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(false)

	return Model{actions: l}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) SetSize(w, h int) Model {
	m.w, m.h = w, h
	m.actions.SetSize(
		ui.Max(20, w-6),
		ui.Max(5, h-6),
	)
	return m
}

/* =========================
   UPDATE
========================= */

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	return m, nil
}

func (m Model) UpdateKeys(k tea.KeyMsg) (Model, tea.Cmd, ui.PromptRequest, bool) {
	switch k.String() {

	case "up", "down":
		var cmd tea.Cmd
		m.actions, cmd = m.actions.Update(k)
		return m, cmd, ui.PromptRequest{}, true

	case "enter":
		i, ok := m.actions.SelectedItem().(actionItem)
		if !ok {
			return m, nil, ui.PromptRequest{}, true
		}

		switch string(i) {

		case "Identify Device":
			return m, status("Detecting ESP device…"), ui.PromptRequest{}, true

		case "Erase Flash", "Flash Firmware":
			m.pendingAction = string(i)
			return m, nil, ui.PromptRequest{
				Active:  true,
				Kind:    ui.PromptConfirmDelete,
				Label:   m.pendingAction + "? (y/n)",
				Initial: "n",
			}, true
		}
	}

	return m, nil, ui.PromptRequest{}, false
}

/* =========================
   PROMPT HANDLING
========================= */

func (m Model) OnPrompt(res ui.PromptResultMsg) (Model, tea.Cmd) {
	if !res.Accepted {
		m.pendingAction = ""
		return m, status("Cancelled")
	}

	switch m.pendingAction {

	case "Erase Flash":
		m.pendingAction = ""
		return m, status("Erasing flash…")

	case "Flash Firmware":
		m.pendingAction = ""
		return m, status("Flashing firmware…")
	}

	return m, nil
}

/* =========================
   VIEW
========================= */

func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}

	title := ui.Accent.Render("Maintenance") +
		ui.Dim.Render(" · Device & firmware tasks")

	body := title + "\n\n" + m.actions.View()

	box := ui.Frame.
		Width(m.w - 2).
		Height(m.h).
		Padding(0, 1)

	return box.Render(
		lipgloss.NewStyle().Render(body),
	)
}

/* =========================
   HELPERS
========================= */

func status(text string) tea.Cmd {
	return func() tea.Msg {
		return ui.StatusMsg{
			Kind: ui.StatusInfo,
			Text: text,
		}
	}
}
