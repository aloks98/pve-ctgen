package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// StatusBar renders a bottom status bar.
type StatusBar struct {
	Width     int
	LeftText  string
	RightText string
}

// View renders the status bar.
func (s StatusBar) View() string {
	left := styles.StatusBarStyle.Render(s.LeftText)
	right := styles.StatusBarStyle.Render(s.RightText)

	gap := s.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		lipgloss.NewStyle().Width(gap).Render(""),
		right,
	)
}
