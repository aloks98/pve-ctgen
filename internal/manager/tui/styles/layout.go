package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

// PaintCanvas restores the canvas colors after nested ANSI styles reset them.
// Lip Gloss wraps each line, but inner SGR resets still return subsequent cells
// to the terminal's defaults. Explicit selection/log colors must be preserved.
func PaintCanvas(content string) string {
	foreground, _, _ := strings.Cut(CanvasStyle.UnsetBackground().Render(" "), " ")
	background, _, _ := strings.Cut(CanvasStyle.UnsetForeground().Render(" "), " ")
	if foreground == "" && background == "" {
		return content
	}
	content = CanvasStyle.Render(content)
	var out strings.Builder
	var pen cellbuf.Style
	parser := ansi.NewParser()
	state := byte(0)
	for len(content) > 0 {
		sequence, width, n, next := ansi.DecodeSequence(content, state, parser)
		if strings.HasPrefix(sequence, "\x1b[") && parser.Command() == 'm' {
			cellbuf.ReadStyle(parser.Params(), &pen)
		}
		if width > 0 {
			if pen.Fg == nil {
				out.WriteString(foreground)
				pen.Fg = White
			}
			if pen.Bg == nil {
				out.WriteString(background)
				pen.Bg = Canvas
			}
		}
		out.WriteString(sequence)
		content, state = content[n:], next
	}
	return out.String()
}

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
