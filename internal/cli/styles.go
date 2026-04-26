package cli

import "github.com/charmbracelet/lipgloss"

var (
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	labelStyle   = lipgloss.NewStyle().Bold(true)
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)
