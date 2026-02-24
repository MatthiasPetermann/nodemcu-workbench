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

	phase string
	done  int
	total int

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
			return m, listenProgress(m.progressCh)
		}
		return m, nil
	case maintenanceDoneMsg:
		m.busy = false
		m.progressCh = nil
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
			return m, nil, ui.PromptRequest{Active: true, Kind: ui.PromptConfirmDelete, Label: "Erase Flash? (y/n)", Initial: "n"}, true
		case "Flash Firmware":
			m.pendingAction = a
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
	if m.pendingAction == "Erase Flash" {
		if strings.ToLower(strings.TrimSpace(res.Value)) != "y" {
			m.pendingAction = ""
			return m, status("Cancelled")
		}
		m.pendingAction = ""
		m.busy, m.phase, m.done, m.total = true, "erase", 0, 1
		return m, tea.Batch(status("Erasing flash…"), runEraseFlash(m.session, m.port, m.baud))
	}
	if m.pendingAction == "Flash Firmware" {
		m.pendingAction = ""
		dir := strings.TrimSpace(res.Value)
		if dir == "" {
			dir = "."
		}
		m.busy, m.phase, m.done, m.total = true, "prepare", 0, 1
		ch := make(chan tea.Msg, 32)
		m.progressCh = ch
		return m, tea.Batch(status("Flashing firmware segments…"), runFlashFirmware(m.session, m.port, m.baud, dir, ch), listenProgress(ch))
	}
	return m, nil
}

func (m Model) View() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	title := ui.Accent.Render("Maintenance") + ui.Dim.Render(" · tiles")
	cards := m.renderTiles()
	bar := m.renderProgress()
	return ui.Frame.Width(m.w-2).Height(m.h).Padding(0, 1).Render(title + "\n\n" + cards + "\n\n" + bar)
}

func (m Model) renderTiles() string {
	// Border + frame composition caused a +1 overflow in each tile.
	cardW := ui.Max(21, (m.w-8)/3-1)
	parts := make([]string, 0, len(m.actions))
	for i, a := range m.actions {
		st := lipgloss.NewStyle().Width(cardW).Height(5).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(ui.T.Border)
		if i == m.cursor {
			st = st.BorderForeground(ui.T.Accent)
		}
		sub := ""
		switch a {
		case "Identify Device":
			sub = "Chip + MAC"
		case "Erase Flash":
			sub = "Full flash erase"
		case "Flash Firmware":
			sub = "0x00000.bin + 0x10000.bin"
		}
		parts = append(parts, st.Render(ui.Accent.Render(a)+"\n"+ui.Dim.Render(sub)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) renderProgress() string {
	phase := m.phase
	if phase == "" {
		phase = "idle"
	}
	if m.total <= 0 {
		return ui.Dim.Render("Phase: " + phase)
	}
	w := ui.Max(10, m.w-20)
	fill := int(float64(m.done) / float64(m.total) * float64(w))
	if fill > w {
		fill = w
	}
	bar := strings.Repeat("█", fill) + strings.Repeat("░", w-fill)
	pct := int(float64(m.done) / float64(m.total) * 100)
	return fmt.Sprintf("Phase: %s\n[%s] %d%%  %d / %d bytes", phase, bar, pct, m.done, m.total)
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

func runFlashFirmware(sess *repl.Session, port string, baud int, dir string, ch chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		segs := []flashSegment{{Offset: 0x0, Path: joinBin(dir, envOr("NODEMCU_BOOT_BIN", "0x00000.bin"))}, {Offset: 0x10000, Path: joinBin(dir, envOr("NODEMCU_APP_BIN", "0x10000.bin"))}}
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
