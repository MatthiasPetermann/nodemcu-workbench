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
	left := Accent.Render(tool) + " " + Dim.Render("·") + " " + Pill.BorderForeground(T.Accent).Render(mode.String())
	right := ""
	if strings.TrimSpace(ctx) != "" {
		right = Dim.Render(ctx)
	}
	space := width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	line := left + strings.Repeat(" ", space) + right

	return lipgloss.NewStyle().
		Width(width).
		Height(1).
		Background(T.BG).
		Padding(0, 1).
		Render(line)
}

func KeyBar(width int, keys []FKey) string {
	var chunks []string
	for _, k := range keys {
		chunks = append(chunks, Accent.Render(k.Key)+" "+Dim.Render(k.Label))
	}
	s := strings.Join(chunks, Dim.Render("  ·  "))
	return lipgloss.NewStyle().
		Width(width).
		Height(1).
		Background(T.BG).
		Padding(0, 1).
		Render(s)
}
