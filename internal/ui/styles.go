package ui

import "github.com/charmbracelet/lipgloss"

var (
	inkColor      = lipgloss.Color("#f8fbff")
	mutedColor    = lipgloss.Color("#6f7a99")
	violetColor   = lipgloss.Color("#7c3aff")
	magentaColor  = lipgloss.Color("#ff4fc3")
	cyanColor     = lipgloss.Color("#32d9ff")
	successColor  = lipgloss.Color("#27ef9f")
	warningColor  = lipgloss.Color("#ffa62b")
	dangerColor   = lipgloss.Color("#ff3b6b")
	panelBgColor  = lipgloss.Color("#050711")
	panelDimColor = lipgloss.Color("#202642")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cyanColor).
			Background(lipgloss.Color("#15112b")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(violetColor)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	dangerStyle = lipgloss.NewStyle().
			Foreground(dangerColor).
			Bold(true)

	accentStyle = lipgloss.NewStyle().
			Foreground(cyanColor).
			Bold(true)

	emptyBarStyle = lipgloss.NewStyle().
			Foreground(panelDimColor)

	panelBorderStyle = lipgloss.NewStyle().
				Foreground(violetColor)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(magentaColor).
			Bold(true)

	panelTextStyle = lipgloss.NewStyle().
			Foreground(inkColor)

	panelShellStyle = lipgloss.NewStyle().
			Foreground(inkColor).
			Background(panelBgColor)
)
