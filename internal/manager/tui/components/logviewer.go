package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// LogViewer is a scrollable log viewport.
type LogViewer struct {
	viewport viewport.Model
	lines    []string
	title    string
	ready    bool
}

// NewLogViewer creates a new LogViewer.
func NewLogViewer(title string) LogViewer {
	return LogViewer{
		title: title,
	}
}

// SetSize sets the viewport dimensions.
func (l *LogViewer) SetSize(w, h int) {
	if !l.ready {
		l.viewport = viewport.New(w, h)
		l.ready = true
	} else {
		l.viewport.Width = w
		l.viewport.Height = h
	}
	l.viewport.SetContent(strings.Join(l.lines, "\n"))
}

// AppendLine adds a line and scrolls to bottom.
func (l *LogViewer) AppendLine(line string) {
	l.lines = append(l.lines, line)
	if l.ready {
		l.viewport.SetContent(strings.Join(l.lines, "\n"))
		l.viewport.GotoBottom()
	}
}

// Clear removes all lines.
func (l *LogViewer) Clear() {
	l.lines = nil
	if l.ready {
		l.viewport.SetContent("")
	}
}

// Update handles key events.
func (l *LogViewer) Update(msg tea.Msg) {
	if l.ready {
		l.viewport, _ = l.viewport.Update(msg)
	}
}

// View renders the log viewer.
func (l *LogViewer) View() string {
	titleBar := styles.TitleStyle.Render(l.title)
	if !l.ready {
		return titleBar + "\n" + styles.MutedStyle.Render("Loading...")
	}

	content := l.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content)
}
