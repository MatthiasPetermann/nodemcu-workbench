package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	BG      lipgloss.Color
	FG      lipgloss.Color
	Border  lipgloss.Color
	Accent  lipgloss.Color
	Accent2 lipgloss.Color
	Warn    lipgloss.Color
	Error   lipgloss.Color
	Dim     lipgloss.Color
}

var T = Theme{
	BG:      lipgloss.Color("#0B1020"),
	FG:      lipgloss.Color("#FFFFFF"),
	Border:  lipgloss.Color("#5CC8FF"),
	Accent:  lipgloss.Color("#B48EFD"),
	Accent2: lipgloss.Color("#7AE582"),
	Warn:    lipgloss.Color("#FFB020"),
	Error:   lipgloss.Color("#FF4D4D"),
	Dim:     lipgloss.Color("#A6B0CF"),
}

var (
	Base   = lipgloss.NewStyle().Background(T.BG).Foreground(T.FG)
	Dim    = lipgloss.NewStyle().Background(T.BG).Foreground(T.Dim)
	Accent = lipgloss.NewStyle().Background(T.BG).Foreground(T.Accent).Bold(true)
	Ok     = lipgloss.NewStyle().Background(T.BG).Foreground(T.Accent2).Bold(true)
	Warn   = lipgloss.NewStyle().Background(T.BG).Foreground(T.Warn).Bold(true)
	Error  = lipgloss.NewStyle().Background(T.BG).Foreground(T.Error).Bold(true)

	Frame = lipgloss.NewStyle().
		Background(T.BG).
		Foreground(T.FG).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(T.Border)

	Rule = lipgloss.NewStyle().Background(T.BG).Foreground(T.Border)

	Pill = lipgloss.NewStyle().
		Padding(0, 1).
		Background(T.BG).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(T.Border)
)
