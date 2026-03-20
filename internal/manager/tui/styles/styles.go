package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Primary   = lipgloss.Color("#7C3AED")
	Secondary = lipgloss.Color("#06B6D4")
	Success   = lipgloss.Color("#22C55E")
	Warning   = lipgloss.Color("#EAB308")
	Error     = lipgloss.Color("#EF4444")
	Muted     = lipgloss.Color("#6B7280")
	BgDark    = lipgloss.Color("#1F2937")
	BgPanel   = lipgloss.Color("#111827")
	White     = lipgloss.Color("#F9FAFB")

	// Title bar
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(White).
			Background(Primary).
			Padding(0, 1)

	// Active/selected items
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	// Status indicators
	SuccessStyle = lipgloss.NewStyle().Foreground(Success)
	WarningStyle = lipgloss.NewStyle().Foreground(Warning)
	ErrorStyle   = lipgloss.NewStyle().Foreground(Error)
	MutedStyle   = lipgloss.NewStyle().Foreground(Muted)

	// Panels
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Padding(1, 2)

	ActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Primary).
				Padding(1, 2)

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Padding(0, 1)

	// Help text
	HelpStyle = lipgloss.NewStyle().
			Foreground(Muted)

	// Table header
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Secondary).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(Muted)

	// Table row
	TableRowStyle = lipgloss.NewStyle()

	// Table selected row
	TableSelectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(White).
				Background(Primary)
)

// StatusIcon returns the appropriate icon for a build status.
func StatusIcon(status string) string {
	switch status {
	case "pending":
		return MutedStyle.Render("○")
	case "running":
		return WarningStyle.Render("◉")
	case "completed":
		return SuccessStyle.Render("✓")
	case "failed":
		return ErrorStyle.Render("✗")
	case "skipped":
		return MutedStyle.Render("–")
	default:
		return MutedStyle.Render("?")
	}
}
