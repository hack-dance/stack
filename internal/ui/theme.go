package ui

import "github.com/charmbracelet/lipgloss"

var (
	AppFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4C7DFF")).
			Padding(1, 2)
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F6C177"))
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CCFD8"))
	SectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#4C7DFF"))
	MutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C8D3F5"))
	CodeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E0DEF4"))
	InfoBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CCFD8"))
	WarnBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F6C177"))
	ErrorBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EB6F92"))
	HealthyBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A6E3A1"))
)
