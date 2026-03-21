package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors — muted palette, btop-inspired
	Primary    = lipgloss.Color("#5F87FF") // soft blue
	Accent     = lipgloss.Color("#87D7AF") // teal-green
	Highlight  = lipgloss.Color("#FFD787") // warm yellow
	Success    = lipgloss.Color("#87D787") // green
	Error      = lipgloss.Color("#FF8787") // red
	Warning    = lipgloss.Color("#FFD787") // yellow
	Muted      = lipgloss.Color("#585858") // dark gray
	Dim        = lipgloss.Color("#808080") // medium gray
	White      = lipgloss.Color("#E4E4E4") // off-white
	BgSelected = lipgloss.Color("#303030") // subtle bg highlight

	// Title bar — full width, inverted
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1C1C1C")).
			Background(Primary).
			Padding(0, 1)

	// Section headers inside panels
	SectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Accent).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(Muted)

	// Active/selected items
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	// Status text styles
	SuccessStyle = lipgloss.NewStyle().Foreground(Success)
	WarningStyle = lipgloss.NewStyle().Foreground(Warning)
	ErrorStyle   = lipgloss.NewStyle().Foreground(Error)
	MutedStyle   = lipgloss.NewStyle().Foreground(Muted)
	DimStyle     = lipgloss.NewStyle().Foreground(Dim)
	BoldStyle    = lipgloss.NewStyle().Bold(true).Foreground(White)
	AccentStyle  = lipgloss.NewStyle().Foreground(Accent)

	// Bordered panel
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Padding(0, 1)

	ActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Primary).
				Padding(0, 1)

	// Status bar — bottom of screen
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(Dim)

	StatusBarKeyStyle = lipgloss.NewStyle().
				Foreground(White).
				Bold(true)

	StatusBarDescStyle = lipgloss.NewStyle().
				Foreground(Dim)

	// Table header
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Accent)

	// Table row
	TableRowStyle = lipgloss.NewStyle().
			Foreground(White)

	// Table selected row
	TableSelectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(Primary).
				Background(BgSelected)

	// Menu item (not selected)
	MenuItemStyle = lipgloss.NewStyle().
			Foreground(White)

	// Menu item description
	MenuDescStyle = lipgloss.NewStyle().
			Foreground(Dim)
)

// StatusTag returns a fixed-width ASCII status tag.
func StatusTag(status string) string {
	switch status {
	case "pending":
		return MutedStyle.Render("[  ]")
	case "running":
		return WarningStyle.Render("[..]")
	case "completed":
		return SuccessStyle.Render("[OK]")
	case "failed":
		return ErrorStyle.Render("[!!]")
	case "skipped":
		return MutedStyle.Render("[--]")
	default:
		return MutedStyle.Render("[??]")
	}
}

// Key renders a keyboard shortcut in the help bar style.
func Key(k string) string {
	return StatusBarKeyStyle.Render(k)
}

// HelpEntry renders a "key:description" pair for the help bar.
func HelpEntry(key, desc string) string {
	return Key(key) + StatusBarDescStyle.Render(":"+desc)
}

// HelpBar renders a full help bar from key-description pairs.
func HelpBar(entries ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, interleave(entries, "  ")...)
}

func interleave(items []string, sep string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, StatusBarDescStyle.Render(sep))
		}
		result = append(result, item)
	}
	return result
}
