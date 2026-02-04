package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#06B6D4")
	successColor   = lipgloss.Color("#10B981")
	warningColor   = lipgloss.Color("#F59E0B")
	errorColor     = lipgloss.Color("#EF4444")
	mutedColor     = lipgloss.Color("#6B7280")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 2)

	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	successBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(successColor).
			Padding(1, 2)

	errorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorColor).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	cursorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	dimStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	coreStyle = lipgloss.NewStyle().
			Foreground(successColor)

	vcpuStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	ccdStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#bb9af7"))

	packageStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7aa2f7"))

	highlightStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(warningColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)
