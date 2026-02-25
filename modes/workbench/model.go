package workbench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nodemcu-workbench/repl"
	"nodemcu-workbench/ui"
)

/* =========================
   DATA TYPES
========================= */

type entry struct {
	Name  string
	IsDir bool
}

type pane struct {
	cwd     string
	entries []entry
	cursor  int
	offset  int
}

type Model struct {
	w, h int

	left, right pane
	activeLeft  bool

	session *repl.Session

	pendingDelete string
}

type uploadDoneMsg struct {
	err error
}

/* =========================
   INIT
========================= */

func New(sess *repl.Session) Model {
	pwd, _ := os.Getwd()

	m := Model{
		activeLeft: true,
		left:       pane{cwd: pwd},
		right:      pane{cwd: "/flash"},
		session:    sess,
	}

	m.left.entries = readDir(pwd)
	m.refreshRemote()

	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) SetSize(w, h int) Model {
	m.w, m.h = w, h
	return m
}

func (m Model) SetSession(sess *repl.Session) Model {
	m.session = sess
	m.refreshRemote()
	return m
}

/* =========================
   UPDATE
========================= */

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uploadDoneMsg:
		if msg.err != nil {
			return m, statusErr(msg.err)
		}
		m.refreshRemote()
		return m, statusInfo("Uploaded")
	}
	return m, nil
}

func (m Model) UpdateKeys(k tea.KeyMsg) (Model, tea.Cmd, ui.PromptRequest, bool) {

	if k.String() == "ctrl+n" && m.activeLeft {
		return m, nil, ui.PromptRequest{
			Active:      true,
			Kind:        ui.PromptNewFile,
			Label:       "New file",
			Placeholder: "e.g. init.lua",
		}, true
	}

	switch k.String() {

	case "tab":
		m.activeLeft = !m.activeLeft
		return m, nil, ui.PromptRequest{}, true

	case "up", "down":
		p := m.activePane()
		if k.String() == "up" && p.cursor > 0 {
			p.cursor--
		}
		if k.String() == "down" && p.cursor < len(p.entries)-1 {
			p.cursor++
		}
		m.adjustOffset(&p)
		m.setActivePane(p)
		return m, nil, ui.PromptRequest{}, true

	case "enter":
		if !m.activeLeft {
			return m, nil, ui.PromptRequest{}, true
		}
		p := m.activePane()
		if len(p.entries) == 0 {
			return m, nil, ui.PromptRequest{}, true
		}
		e := p.entries[p.cursor]
		if e.IsDir {
			p.cwd = filepath.Join(p.cwd, e.Name)
			p.entries = readDir(p.cwd)
			p.cursor, p.offset = 0, 0
			m.setActivePane(p)
		}
		return m, nil, ui.PromptRequest{}, true

	case "backspace":
		if !m.activeLeft {
			return m, nil, ui.PromptRequest{}, true
		}
		p := m.activePane()
		parent := filepath.Dir(p.cwd)
		if parent != p.cwd {
			p.cwd = parent
			p.entries = readDir(parent)
			p.cursor, p.offset = 0, 0
			m.setActivePane(p)
		}
		return m, nil, ui.PromptRequest{}, true

	case "f2":
		m.left.entries = readDir(m.left.cwd)
		m.refreshRemote()
		return m, statusInfo("Refreshed"), ui.PromptRequest{}, true

	case "f4":
		if !m.activeLeft {
			return m, nil, ui.PromptRequest{}, true
		}
		path, err := m.selectedPath()
		if err != nil {
			return m, statusErr(err), ui.PromptRequest{}, true
		}
		return m, execNano(path), ui.PromptRequest{}, true

	case "f5":
		if m.activeLeft {
			src, err := m.selectedPath()
			if err != nil {
				return m, statusErr(err), ui.PromptRequest{}, true
			}
			return m, m.uploadCmd(src), ui.PromptRequest{}, true
		}
		return m, nil, ui.PromptRequest{}, true

	case "f6":
		if !m.activeLeft {
			return m, nil, ui.PromptRequest{}, true
		}
		path, err := m.selectedPath()
		if err != nil {
			return m, statusErr(err), ui.PromptRequest{}, true
		}
		return m, nil, ui.PromptRequest{
			Active:      true,
			Kind:        ui.PromptRename,
			Label:       "Rename to",
			Initial:     filepath.Base(path),
			Placeholder: filepath.Base(path),
		}, true

	case "f7":
		if !m.activeLeft {
			return m, nil, ui.PromptRequest{}, true
		}
		return m, nil, ui.PromptRequest{
			Active:      true,
			Kind:        ui.PromptNewDir,
			Label:       "New directory",
			Placeholder: "e.g. firmware",
		}, true

	case "f8":
		p := m.activePane()
		if len(p.entries) == 0 {
			return m, nil, ui.PromptRequest{}, true
		}
		m.pendingDelete = p.entries[p.cursor].Name
		return m, nil, ui.PromptRequest{
			Active:  true,
			Kind:    ui.PromptConfirmDelete,
			Label:   "Delete? (y/n)",
			Initial: "n",
		}, true
	}

	return m, nil, ui.PromptRequest{}, false
}

/* =========================
   PROMPT HANDLING
========================= */

func (m Model) OnPrompt(res ui.PromptResultMsg) (Model, tea.Cmd) {
	if !res.Accepted {
		m.pendingDelete = ""
		return m, statusInfo("Cancelled")
	}

	switch res.Kind {

	case ui.PromptNewFile:
		path := filepath.Join(m.left.cwd, res.Value)
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			return m, statusErr(err)
		}
		m.left.entries = readDir(m.left.cwd)
		return m, tea.Batch(statusInfo("Created file"), execNano(path))

	case ui.PromptRename:
		oldPath, err := m.selectedPath()
		if err != nil {
			return m, statusErr(err)
		}
		newPath := filepath.Join(filepath.Dir(oldPath), res.Value)
		if err := os.Rename(oldPath, newPath); err != nil {
			return m, statusErr(err)
		}
		m.left.entries = readDir(m.left.cwd)
		return m, statusInfo("Renamed")

	case ui.PromptNewDir:
		dir := filepath.Join(m.left.cwd, res.Value)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return m, statusErr(err)
		}
		m.left.entries = readDir(m.left.cwd)
		return m, statusInfo("Directory created")

	case ui.PromptConfirmDelete:
		if m.activeLeft {
			if err := os.RemoveAll(filepath.Join(m.left.cwd, m.pendingDelete)); err != nil {
				return m, statusErr(err)
			}
			m.left.entries = readDir(m.left.cwd)
			return m, statusInfo("Deleted")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if m.session == nil {
			return m, statusErr(errors.New("not connected"))
		}
		if err := m.session.Remove(ctx, m.pendingDelete); err != nil {
			return m, statusErr(err)
		}
		m.refreshRemote()
		return m, statusInfo("Remote file deleted")
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

	inner := (m.w - 4) / 2

	left := m.renderPane("LOCAL", m.left, m.activeLeft, inner, m.h)
	right := m.renderPane("REMOTE", m.right, !m.activeLeft, inner, m.h)

	return lipgloss.NewStyle().
		Width(m.w).
		Height(m.h).
		Background(ui.T.BG).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
}

func (m Model) renderPane(label string, p pane, active bool, width, height int) string {
	title := ui.Accent.Render(label) + ui.Dim.Render("  "+p.cwd)
	sep := ui.Rule.Render(strings.Repeat("─", width-4))

	bodyHeight := height - 4
	start := p.offset
	end := ui.Min(len(p.entries), start+bodyHeight)

	lines := []string{}
	for i := start; i < end; i++ {
		e := p.entries[i]
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		line := "  " + name
		if i == p.cursor {
			line = ui.Ok.Render("▸ " + name)
		}
		lines = append(lines, line)
	}

	box := ui.Frame.Width(width).Height(height).Padding(0, 1)
	if active {
		box = box.BorderForeground(ui.T.Accent)
	}
	return box.Render(title + "\n" + sep + "\n" + strings.Join(lines, "\n"))
}

/* =========================
   HELPERS
========================= */

func (m *Model) refreshRemote() {
	if m.session == nil {
		m.right.entries = nil
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	files, err := m.session.RemoteList(ctx)
	if err != nil {
		m.right.entries = nil
		return
	}

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]entry, 0, len(names))
	for _, n := range names {
		out = append(out, entry{Name: n})
	}
	m.right.entries = out
	m.right.cursor, m.right.offset = 0, 0
}

func (m Model) activePane() pane {
	if m.activeLeft {
		return m.left
	}
	return m.right
}

func (m *Model) setActivePane(p pane) {
	if m.activeLeft {
		m.left = p
	} else {
		m.right = p
	}
}

func (m Model) selectedPath() (string, error) {
	p := m.activePane()
	if len(p.entries) == 0 {
		return "", errors.New("no selection")
	}
	return filepath.Join(p.cwd, p.entries[p.cursor].Name), nil
}

/* =========================
   COMMANDS
========================= */

func execNano(path string) tea.Cmd {
	cmd := exec.Command("nano", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return ui.StatusMsg{Kind: ui.StatusError, Text: err.Error()}
		}
		return ui.StatusMsg{Kind: ui.StatusInfo, Text: "Editor closed"}
	})
}

func (m Model) uploadCmd(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return uploadDoneMsg{err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if m.session == nil {
			return uploadDoneMsg{err: errors.New("not connected")}
		}

		err = m.session.WriteFile(ctx, filepath.Base(path), data, nil)
		if err != nil {
			return uploadDoneMsg{err: err}
		}
		return uploadDoneMsg{}
	}
}

/* =========================
   FILE OPS
========================= */

func readDir(dir string) []entry {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type tmp struct {
		n string
		d bool
	}
	t := []tmp{}
	for _, d := range des {
		t = append(t, tmp{d.Name(), d.IsDir()})
	}
	sort.Slice(t, func(i, j int) bool {
		if t[i].d != t[j].d {
			return t[i].d
		}
		return strings.ToLower(t[i].n) < strings.ToLower(t[j].n)
	})
	out := []entry{}
	for _, e := range t {
		out = append(out, entry{e.n, e.d})
	}
	return out
}

/* =========================
   STATUS HELPERS
========================= */

func statusInfo(s string) tea.Cmd {
	return func() tea.Msg { return ui.StatusMsg{Kind: ui.StatusInfo, Text: s} }
}
func statusErr(err error) tea.Cmd {
	return func() tea.Msg { return ui.StatusMsg{Kind: ui.StatusError, Text: err.Error()} }
}

/* =========================
   SCROLL LOGIC
========================= */

func (m Model) adjustOffset(p *pane) {
	visible := ui.Max(1, m.h-4)

	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+visible {
		p.offset = p.cursor - visible + 1
	}
}
