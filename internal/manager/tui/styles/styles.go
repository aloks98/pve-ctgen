package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Workspace artboard palette, with a corresponding light theme.
	Canvas      = lipgloss.AdaptiveColor{Light: "#F6F7F9", Dark: "#11151C"}
	Border      = lipgloss.AdaptiveColor{Light: "#BAC5D1", Dark: "#3B485B"}
	CanvasStyle = lipgloss.NewStyle().Foreground(White).Background(Canvas)
	RuleStyle   = lipgloss.NewStyle().Foreground(Border)
	Primary     = lipgloss.AdaptiveColor{Light: "#2855b8", Dark: "#90AEFF"} // soft blue
	Accent      = lipgloss.AdaptiveColor{Light: "#286447", Dark: "#8DD7B0"} // teal-green
	Highlight   = lipgloss.AdaptiveColor{Light: "#815900", Dark: "#F0CB82"} // warm yellow
	Success     = lipgloss.AdaptiveColor{Light: "#286447", Dark: "#8DD7B0"} // green
	Error       = lipgloss.AdaptiveColor{Light: "#A22D3D", Dark: "#F59AAB"} // red
	Warning     = lipgloss.AdaptiveColor{Light: "#815900", Dark: "#F0CB82"} // yellow
	Muted       = lipgloss.AdaptiveColor{Light: "#697586", Dark: "#8695A8"} // dark gray
	Dim         = lipgloss.AdaptiveColor{Light: "#536276", Dark: "#96A4B6"} // medium gray
	White       = lipgloss.AdaptiveColor{Light: "#263345", Dark: "#D9E2EE"} // off-white
	BgSelected  = lipgloss.AdaptiveColor{Light: "#DFE8FF", Dark: "#25354F"} // subtle bg highlight

	// Title bar — full width, inverted
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary).
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
	TableHeaderStyle = lipgloss.NewStyle().Foreground(Dim)

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

// HelpEntry renders a key and description pair for the help bar.
func HelpEntry(key, desc string) string {
	return Key(key) + StatusBarDescStyle.Render(" "+desc)
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
