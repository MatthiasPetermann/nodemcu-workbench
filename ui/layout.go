package ui

import "github.com/charmbracelet/lipgloss"

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func JoinVertical(parts ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
