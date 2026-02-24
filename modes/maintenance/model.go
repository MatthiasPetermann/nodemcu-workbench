package maintenance

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nodemcu-workbench/ui"
)

type actionItem string

func (i actionItem) Title() string       { return string(i) }
func (i actionItem) Description() string { return "" }
func (i actionItem) FilterValue() string { return string(i) }

type Model struct {
	w, h int

	actions list.Model

	pendingAction string
	port          string
	baud          int
	busy          bool
}

type maintenanceDoneMsg struct {
	text string
	err  error
}

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

	port := os.Getenv("NODEMCU_PORT")
	if port == "" {
		port = "/dev/ttyUSB0"
	}

	return Model{actions: l, port: port, baud: 115200}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) SetSize(w, h int) Model {
	m.w, m.h = w, h
	m.actions.SetSize(ui.Max(20, w-6), ui.Max(5, h-6))
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case maintenanceDoneMsg:
		m.busy = false
		if msg.err != nil {
			return m, statusErr(msg.err.Error())
		}
		return m, status(msg.text)
	}
	return m, nil
}

func (m Model) UpdateKeys(k tea.KeyMsg) (Model, tea.Cmd, ui.PromptRequest, bool) {
	if m.busy {
		return m, statusWarn("Maintenance task is running…"), ui.PromptRequest{}, true
	}

	switch k.String() {
	case "up", "down":
		var cmd tea.Cmd
		m.actions, cmd = m.actions.Update(k)
		return m, cmd, ui.PromptRequest{}, true
	case "enter", "f5", "f8":
		i, ok := m.actions.SelectedItem().(actionItem)
		if !ok {
			return m, nil, ui.PromptRequest{}, true
		}
		switch string(i) {
		case "Identify Device":
			m.busy = true
			return m, tea.Batch(status("Detecting ESP device…"), runIdentify(m.port, m.baud)), ui.PromptRequest{}, true
		case "Erase Flash":
			m.pendingAction = string(i)
			return m, nil, ui.PromptRequest{Active: true, Kind: ui.PromptConfirmDelete, Label: m.pendingAction + "? (y/n)", Initial: "n"}, true
		case "Flash Firmware":
			m.pendingAction = string(i)
			return m, nil, ui.PromptRequest{Active: true, Kind: ui.PromptNewFile, Label: "Firmware directory", Placeholder: "e.g. ./firmware", Initial: os.Getenv("NODEMCU_FIRMWARE_DIR")}, true
		}
	}

	return m, nil, ui.PromptRequest{}, false
}

func (m Model) OnPrompt(res ui.PromptResultMsg) (Model, tea.Cmd) {
	if !res.Accepted {
		m.pendingAction = ""
		return m, status("Cancelled")
	}

	switch m.pendingAction {
	case "Erase Flash":
		if strings.ToLower(strings.TrimSpace(res.Value)) != "y" {
			m.pendingAction = ""
			return m, status("Cancelled")
		}
		m.pendingAction = ""
		m.busy = true
		return m, tea.Batch(status("Erasing flash…"), runEraseFlash(m.port, m.baud))

	case "Flash Firmware":
		m.pendingAction = ""
		dir := strings.TrimSpace(res.Value)
		if dir == "" {
			dir = "."
		}
		m.busy = true
		return m, tea.Batch(status("Flashing firmware segments…"), runFlashFirmware(m.port, m.baud, dir))
	}

	return m, nil
}

func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}

	detail := " · Device & firmware tasks"
	if m.busy {
		detail = " · busy"
	}
	title := ui.Accent.Render("Maintenance") + ui.Dim.Render(detail)
	body := title + "\n\n" + m.actions.View()
	box := ui.Frame.Width(m.w-2).Height(m.h).Padding(0, 1)
	return box.Render(lipgloss.NewStyle().Render(body))
}

func runIdentify(port string, baud int) tea.Cmd {
	return func() tea.Msg {
		c, err := openESPClient(port, baud)
		if err != nil {
			return maintenanceDoneMsg{err: err}
		}
		defer c.Close()

		c.enterBootloader()
		if err := c.sync(); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		info, err := c.identify()
		if err != nil {
			return maintenanceDoneMsg{err: err}
		}
		return maintenanceDoneMsg{text: fmt.Sprintf("%s chip=0x%06x mac=%s magic=0x%08x", info.Chip, info.ChipID, info.Mac, info.MagicValue)}
	}
}

func runEraseFlash(port string, baud int) tea.Cmd {
	return func() tea.Msg {
		c, err := openESPClient(port, baud)
		if err != nil {
			return maintenanceDoneMsg{err: err}
		}
		defer c.Close()
		c.enterBootloader()
		if err := c.sync(); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		if err := c.eraseFlash(); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		return maintenanceDoneMsg{text: "Flash erased"}
	}
}

func runFlashFirmware(port string, baud int, dir string) tea.Cmd {
	return func() tea.Msg {
		c, err := openESPClient(port, baud)
		if err != nil {
			return maintenanceDoneMsg{err: err}
		}
		defer c.Close()
		c.enterBootloader()
		if err := c.sync(); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		segs := []flashSegment{
			{Offset: 0x0, Path: joinBin(dir, envOr("NODEMCU_BOOT_BIN", "0x00000.bin"))},
			{Offset: 0x10000, Path: joinBin(dir, envOr("NODEMCU_APP_BIN", "0x10000.bin"))},
		}
		if err := c.flashImages(segs); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		return maintenanceDoneMsg{text: "Firmware flashed (0x00000.bin@0x0, 0x10000.bin@0x10000)"}
	}
}

func joinBin(dir, file string) string {
	if strings.TrimSpace(dir) == "" || dir == "." {
		return file
	}
	return dir + string(os.PathSeparator) + file
}

func envOr(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func status(text string) tea.Cmd {
	return func() tea.Msg { return ui.StatusMsg{Kind: ui.StatusInfo, Text: text} }
}

func statusWarn(text string) tea.Cmd {
	return func() tea.Msg { return ui.StatusMsg{Kind: ui.StatusWarn, Text: text} }
}

func statusErr(text string) tea.Cmd {
	return func() tea.Msg { return ui.StatusMsg{Kind: ui.StatusError, Text: text} }
}
