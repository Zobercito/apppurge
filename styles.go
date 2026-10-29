package main

import "github.com/charmbracelet/lipgloss"

var (
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A8DADC"))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#06D6A0"))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#495057")).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8DADC")).
			Italic(true)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#06D6A0"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#EF476F"))

	warningTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFD166"))

	warningStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD166")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD166")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06D6A0")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C757D"))

	highlightStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD166"))

	sourceAPT = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4ECDC4")).
			Bold(true)

	sourcePacman = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1793D1")). // Arch blue
			Bold(true)

	sourceFlatpak = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	sourceSnap = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F97316")).
			Bold(true)

	sourceAppImage = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EC4899")).
			Bold(true)

	sourceWebApp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FACC15")).
			Bold(true)

	sourceAll = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
)
