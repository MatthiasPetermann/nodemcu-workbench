package maintenance

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.bug.st/serial"

	"nodemcu-workbench/repl"
	"nodemcu-workbench/ui"
)

type Model struct {
	w, h int

	actions []string
	cursor  int

	pendingAction string
	port          string
	baud          int
	busy          bool
	session       *repl.Session

	phase      string
	done       int
	total      int
	progressCh <-chan tea.Msg
}

type maintenanceDoneMsg struct {
	text string
	err  error
}

type maintenanceProgressMsg struct {
	phase string
	done  int
	total int
}

func New(sess *repl.Session) Model {
	port := os.Getenv("NODEMCU_PORT")
	if port == "" {
		port = "/dev/ttyUSB0"
	}
	return Model{
		actions: []string{"Identify Device", "Erase Flash", "Flash Firmware"},
		port:    port,
		baud:    115200,
		session: sess,
		phase:   "idle",
	}
}

func (m Model) SetSession(sess *repl.Session) Model { m.session = sess; return m }
func (m Model) Init() tea.Cmd                       { return nil }
func (m Model) SetSize(w, h int) Model              { m.w, m.h = w, h; return m }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case maintenanceProgressMsg:
		m.phase, m.done, m.total = msg.phase, msg.done, msg.total
		if m.progressCh != nil {
			return m, tea.Batch(statusProgress(true, m.phase, m.done, m.total), listenProgress(m.progressCh))
		}
		return m, nil
	case maintenanceDoneMsg:
		m.busy = false
		m.progressCh = nil
		m.phase, m.done, m.total = "idle", 0, 0
		clear := statusProgress(false, "", 0, 0)
		if msg.err != nil {
			return m, tea.Batch(clear, statusErr(msg.err.Error()))
		}
		return m, tea.Batch(clear, status(msg.text))
	}
	return m, nil
}

func (m Model) UpdateKeys(k tea.KeyMsg) (Model, tea.Cmd, ui.PromptRequest, bool) {
	if m.busy {
		return m, statusWarn("Maintenance task is running…"), ui.PromptRequest{}, true
	}

	switch k.String() {
	case "left", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil, ui.PromptRequest{}, true
	case "right", "down":
		if m.cursor < len(m.actions)-1 {
			m.cursor++
		}
		return m, nil, ui.PromptRequest{}, true
	case "enter", "f5", "f8":
		a := m.actions[m.cursor]
		switch a {
		case "Identify Device":
			m.busy, m.phase, m.done, m.total = true, "identify", 0, 1
			return m, tea.Batch(status("Detecting ESP device…"), runIdentify(m.session, m.port, m.baud)), ui.PromptRequest{}, true
		case "Erase Flash":
			m.pendingAction = a
			return m, nil, ui.PromptRequest{Active: true, Kind: ui.PromptConfirmDelete, Label: "Erase Flash bestätigen"}, true
		case "Flash Firmware":
			m.pendingAction = a
			return m, nil, ui.PromptRequest{Active: true, Kind: ui.PromptConfirmDelete, Label: "Flash Firmware bestätigen"}, true
		}
	}
	return m, nil, ui.PromptRequest{}, false
}

func (m Model) OnPrompt(res ui.PromptResultMsg) (Model, tea.Cmd) {
	if !res.Accepted {
		m.pendingAction = ""
		return m, status("Cancelled")
	}
	if m.pendingAction == "Erase Flash" {
		m.pendingAction = ""
		m.busy, m.phase, m.done, m.total = true, "erase", 0, 1
		return m, tea.Batch(status("Erasing flash…"), runEraseFlash(m.session, m.port, m.baud))
	}
	if m.pendingAction == "Flash Firmware" {
		m.pendingAction = ""
		m.busy, m.phase, m.done, m.total = true, "prepare", 0, 1
		ch := make(chan tea.Msg, 32)
		m.progressCh = ch
		return m, tea.Batch(status("Flashing embedded firmware…"), statusProgress(true, "prepare", 0, 1), runFlashFirmware(m.session, m.port, m.baud, ch), listenProgress(ch))
	}
	return m, nil
}

func (m Model) View() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	title := ui.Accent.Render("Maintenance") + ui.Dim.Render(" · vertical tiles")
	cards := m.renderTiles()
	body := title + "\n\n" + cards
	return ui.Frame.Width(m.w-2).Height(m.h).Padding(0, 1).Render(body)
}

func (m Model) renderTiles() string {
	cardW := ui.Max(28, m.w-8)
	tileH := ui.Max(5, (m.h-10)/len(m.actions))
	parts := make([]string, 0, len(m.actions))
	for i, a := range m.actions {
		st := lipgloss.NewStyle().Width(cardW).Height(tileH).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(ui.T.Border)
		if i == m.cursor {
			st = st.BorderForeground(ui.T.Accent)
		}
		sub, art := "", ""
		switch a {
		case "Identify Device":
			sub = "Chip + MAC"
			art = "     ┌─────────┐\n ─┬──┤    ?    ├──┬─\n ─┴──┤         ├──┴─\n     └─────────┘"
		case "Erase Flash":
			sub = "Full flash erase"
			art = "     ┌─────────┐\n ─┬──┤  XXXXX  ├──┬─\n ─┴──┤  XXXXX  ├──┴─\n     └─────────┘"
		case "Flash Firmware":
			sub = "0x00000.bin + 0x10000.bin"
			art = "   ░░░░  ──▶\n     ┌─────────┐\n ─┬──┤  ░░░░░  ├──┬─\n ─┴──┤  ░░░░░  ├──┴─\n     └─────────┘"
		}
		left := lipgloss.NewStyle().Width(cardW - 24).Render(ui.Accent.Render(a) + "\n" + ui.Dim.Render(sub))
		right := lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Background(ui.T.BG).Render(ui.Base.Render(art))
		parts = append(parts, st.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func listenProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func runIdentify(sess *repl.Session, port string, baud int) tea.Cmd {
	return func() tea.Msg {
		if sess != nil {
			var info deviceInfo
			err := sess.WithExclusivePort(func(sp serial.Port) error {
				c := &espClient{port: sp, owned: false}
				c.enterBootloader()
				if err := c.sync(); err != nil {
					return err
				}
				d, err := c.identify()
				if err != nil {
					return err
				}
				info = d
				return nil
			})
			if err != nil {
				return maintenanceDoneMsg{err: err}
			}
			return maintenanceDoneMsg{text: fmt.Sprintf("%s chip=0x%06x mac=%s magic=0x%08x", info.Chip, info.ChipID, info.Mac, info.MagicValue)}
		}
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

func runEraseFlash(sess *repl.Session, port string, baud int) tea.Cmd {
	return func() tea.Msg {
		if sess != nil {
			err := sess.WithExclusivePort(func(sp serial.Port) error {
				c := &espClient{port: sp, owned: false}
				c.enterBootloader()
				if err := c.sync(); err != nil {
					return err
				}
				if err := c.eraseFlash(); err != nil {
					return err
				}
				c.hardReset()
				return nil
			})
			if err != nil {
				return maintenanceDoneMsg{err: err}
			}
			return maintenanceDoneMsg{text: "Flash erased"}
		}
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
		c.hardReset()
		return maintenanceDoneMsg{text: "Flash erased"}
	}
}

func runFlashFirmware(sess *repl.Session, port string, baud int, ch chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		segs := []flashSegment{{Offset: 0x0, Path: envOr("NODEMCU_BOOT_BIN", "0x00000.bin")}, {Offset: 0x10000, Path: envOr("NODEMCU_APP_BIN", "0x10000.bin")}}
		report := func(phase string, done, total int) {
			ch <- maintenanceProgressMsg{phase: phase, done: done, total: total}
		}
		if sess != nil {
			err := sess.WithExclusivePort(func(sp serial.Port) error {
				c := &espClient{port: sp, owned: false}
				c.enterBootloader()
				if err := c.sync(); err != nil {
					return err
				}
				if err := c.flashImages(segs, report); err != nil {
					return err
				}
				c.hardReset()
				return nil
			})
			if err != nil {
				return maintenanceDoneMsg{err: err}
			}
			return maintenanceDoneMsg{text: "Firmware flashed (0x00000.bin@0x0, 0x10000.bin@0x10000)"}
		}
		c, err := openESPClient(port, baud)
		if err != nil {
			return maintenanceDoneMsg{err: err}
		}
		defer c.Close()
		c.enterBootloader()
		if err := c.sync(); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		if err := c.flashImages(segs, report); err != nil {
			return maintenanceDoneMsg{err: err}
		}
		c.hardReset()
		return maintenanceDoneMsg{text: "Firmware flashed (0x00000.bin@0x0, 0x10000.bin@0x10000)"}
	}
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

func statusProgress(active bool, phase string, done, total int) tea.Cmd {
	return func() tea.Msg { return ui.ProgressMsg{Active: active, Phase: phase, Done: done, Total: total} }
}
