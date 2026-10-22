package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// LogViewer is a scrollable log viewport.
type LogViewer struct {
	viewport viewport.Model
	lines    []string
	title    string
	ready    bool
	follow   bool
}

// NewLogViewer creates a new LogViewer.
func NewLogViewer(title string) LogViewer {
	return LogViewer{
		title:  title,
		follow: true,
	}
}

// SetSize sets the viewport dimensions.
func (l *LogViewer) SetSize(w, h int) {
	w = max(1, w)
	h = max(1, h)
	if !l.ready {
		l.viewport = viewport.New(w, h)
		l.ready = true
	} else {
		l.viewport.Width = w
		l.viewport.Height = h
	}
	l.refresh()
}

// AppendLine adds a line and scrolls to bottom.
func (l *LogViewer) AppendLine(line string) {
	l.lines = append(l.lines, strings.Split(ansi.Strip(line), "\n")...)
	// The full log is persisted by the coordinator; bound only the live buffer.
	if len(l.lines) > 5000 {
		l.lines = append([]string(nil), l.lines[len(l.lines)-5000:]...)
	}
	l.refresh()
}
func (l *LogViewer) refresh() {
	if !l.ready {
		return
	}
	l.viewport.SetContent(ansi.Hardwrap(strings.Join(l.lines, "\n"), max(1, l.viewport.Width), true))
	if l.follow {
		l.viewport.GotoBottom()
	}
}

// Clear removes all lines.
func (l *LogViewer) Clear() {
	l.lines = nil
	l.follow = true
	if l.ready {
		l.viewport.SetContent("")
	}
}

// Update handles key events.
func (l *LogViewer) Update(msg tea.Msg) {
	if !l.ready {
		return
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "f", "end", "G":
			l.follow = true
			l.viewport.GotoBottom()
			return
		case "up", "k", "pgup", "home", "g":
			l.follow = false
		}
	}
	l.viewport, _ = l.viewport.Update(msg)
}

// View renders the log viewer.
func (l *LogViewer) View() string {
	state := "FOLLOWING"
	if !l.follow {
		state = "PAUSED - f follow"
	}
	titleBar := styles.DimStyle.Render(l.title + " / " + state)
	if !l.ready {
		return titleBar + "\n" + styles.MutedStyle.Render("Loading...")
	}

	content := l.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content)
}
