package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Rule draws a quiet, continuous separator in terminal cells.
func Rule(width int) string {
	return RuleStyle.Render(strings.Repeat("─", max(0, width)))
}

// PageHeader gives list screens the same title, secondary text and action row.
func PageHeader(title, subtitle, action string, width int) string {
	heading := BoldStyle.Render(title)
	if action != "" && width >= lipgloss.Width(title)+lipgloss.Width(action)+6 {
		chip := TableSelectedRowStyle.Render(" " + action + " ")
		heading += strings.Repeat(" ", max(1, width-lipgloss.Width(heading)-lipgloss.Width(chip))) + chip
	}
	return Fit(heading, width, 1) + "\n" + Fit(DimStyle.Render(subtitle), width, 1) + "\n\n"
}

// Fit bounds terminal output by display cells, preserving ANSI sequences and
// padding to an exact rectangle. Width is never measured in UTF-8 bytes.
func Fit(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	result := make([]string, height)
	for i := range result {
		if i < len(lines) {
			result[i] = ansi.Truncate(lines[i], width, "")
		}
		result[i] += strings.Repeat(" ", max(0, width-lipgloss.Width(result[i])))
	}
	return strings.Join(result, "\n")
}
