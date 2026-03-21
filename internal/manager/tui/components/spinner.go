package components

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// Spinner wraps the bubbles spinner with our styling.
type Spinner struct {
	Model   spinner.Model
	Label   string
	Visible bool
}

// NewSpinner creates a line-style spinner (-\|/).
func NewSpinner(label string) Spinner {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"-", "\\", "|", "/"},
		FPS:    time.Second / 10, // ~100ms per frame like ora
	}
	s.Style = lipgloss.NewStyle().Foreground(styles.Warning)
	return Spinner{Model: s, Label: label, Visible: true}
}

// Update updates the spinner.
func (s *Spinner) Update(msg tea.Msg) tea.Cmd {
	if !s.Visible {
		return nil
	}
	var cmd tea.Cmd
	s.Model, cmd = s.Model.Update(msg)
	return cmd
}

// Tick returns the initial tick command.
func (s *Spinner) Tick() tea.Cmd {
	return s.Model.Tick
}

// View renders the spinner with label.
func (s Spinner) View() string {
	if !s.Visible {
		return ""
	}
	return s.Model.View() + " " + styles.DimStyle.Render(s.Label)
}
