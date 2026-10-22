package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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
