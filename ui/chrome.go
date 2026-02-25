package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type FKey struct {
	Key   string
	Label string
}

func Header(width int, tool string, mode Mode, ctx string) string {
	innerW := Max(1, width-2)
	left := Accent.Render(tool) + " " + Dim.Render("·") + " " + Pill.BorderForeground(T.Accent).Render(mode.String())
	right := ""
	if strings.TrimSpace(ctx) != "" {
		right = Dim.Render(ctx)
	}
	space := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	line := left + strings.Repeat(" ", space) + right
	line = lipgloss.NewStyle().Width(innerW).MaxWidth(innerW).Render(line)

	return lipgloss.NewStyle().
		Width(width).
		Height(1).
		Background(T.BG).
		Padding(0, 1).
		Render(line)
}

func KeyBar(width int, keys []FKey) string {
	innerW := Max(1, width-2)
	rows := 2
	cols := (len(keys) + rows - 1) / rows
	if cols < 1 {
		cols = 1
	}
	colW := Max(1, innerW/cols)

	renderCell := func(k FKey) string {
		text := ""
		if strings.TrimSpace(k.Key) != "" || strings.TrimSpace(k.Label) != "" {
			text = Accent.Render(k.Key)
			if strings.TrimSpace(k.Label) != "" {
				text += " " + Dim.Render(k.Label)
			}
		}
		return lipgloss.NewStyle().Width(colW).MaxWidth(colW).Render(text)
	}

	cells := make([]FKey, rows*cols)
	copy(cells, keys)

	lines := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		parts := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			parts = append(parts, renderCell(cells[r*cols+c]))
		}
		line := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
		lines = append(lines, lipgloss.NewStyle().Width(innerW).MaxWidth(innerW).Render(line))
	}

	text := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(width).
		Height(2).
		Background(T.BG).
		Padding(0, 1).
		Render(text)
}
