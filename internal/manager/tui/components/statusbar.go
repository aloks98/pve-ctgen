package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// StatusBar renders a bottom status bar with breadcrumb + help keys.
type StatusBar struct {
	Width      int
	Breadcrumb string // e.g. "Home > Nodes"
	HelpKeys   string // rendered help entries
	RightText  string // e.g. build status indicator
}

// View renders the status bar.
func (s StatusBar) View() string {
	bar := lipgloss.NewStyle().
		Width(s.Width).
		Background(lipgloss.Color("#1C1C1C")).
		Padding(0, 1)

	left := styles.AccentStyle.Render(s.Breadcrumb)
	right := s.RightText

	helpWidth := s.Width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	help := ""
	if helpWidth > 10 {
		help = lipgloss.NewStyle().
			Width(helpWidth).
			Align(lipgloss.Center).
			Render(s.HelpKeys)
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, help, right)
	return bar.Render(content)
}
