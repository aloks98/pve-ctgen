package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// BuildEventMsg wraps a gRPC build event for the TUI.
type BuildEventMsg struct {
	Event *pb.BuildEvent
}

// BuildDoneMsg signals that the event channel has been closed (all builds finished).
type BuildDoneMsg struct{}

type stepStatus struct {
	name   string
	status string
}

type imageStatus struct {
	name      string
	status    string
	steps     []stepStatus
	collapsed bool
}

// BuildProgressModel shows live build progress.
type BuildProgressModel struct {
	images       []imageStatus
	logViewer    components.LogViewer
	events       <-chan *pb.BuildEvent
	spinner      spinner.Model
	currentStep  string
	currentCmd   string
	Done         bool
	Backgrounded bool
	succeeded    int
	failed       int
	width        int
	height       int
}

// NewBuildProgressModel creates a new BuildProgressModel.
func NewBuildProgressModel() BuildProgressModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"-", "\\", "|", "/"},
		FPS:    time.Second / 10,
	}
	return BuildProgressModel{
		logViewer: components.NewLogViewer("Build Output"),
		spinner:   s,
	}
}

// SetSize sets the view dimensions.
func (m *BuildProgressModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.logViewer.SetSize(w/2, h-6)
}

// Reset clears state for a new build and sets the event channel.
func (m *BuildProgressModel) Reset(events <-chan *pb.BuildEvent) {
	m.images = nil
	m.events = events
	m.currentStep = ""
	m.currentCmd = ""
	m.Done = false
	m.Backgrounded = false
	m.succeeded = 0
	m.failed = 0
	m.logViewer.Clear()
}

// Running returns true if a build is in progress (not done, has events channel).
func (m *BuildProgressModel) Running() bool {
	return m.events != nil && !m.Done
}

// Init returns a command to start listening for events and the spinner.
func (m *BuildProgressModel) Init() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return tea.Batch(listenForEvent(m.events), m.spinner.Tick)
}

func listenForEvent(events <-chan *pb.BuildEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return BuildDoneMsg{}
		}
		return BuildEventMsg{Event: event}
	}
}

// Update handles events.
func (m BuildProgressModel) Update(msg tea.Msg) (BuildProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case BuildEventMsg:
		e := msg.Event
		switch e.Type {
		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED:
			m.currentStep = e.StepName
			m.currentCmd = ""
			if len(m.images) == 0 || m.images[len(m.images)-1].status == "completed" || m.images[len(m.images)-1].status == "failed" {
				name := e.StepName
				if e.BuildId != "" && len(e.BuildId) >= 8 {
					name = fmt.Sprintf("Build %s", e.BuildId[:8])
				}
				m.images = append(m.images, imageStatus{name: name, status: "running"})
				// Clear log for the new build
				m.logViewer.Clear()
				m.logViewer.AppendLine(fmt.Sprintf("--- %s ---", name))
			}
			img := &m.images[len(m.images)-1]
			img.steps = append(img.steps, stepStatus{name: e.StepName, status: "running"})

		case pb.BuildEventType_BUILD_EVENT_TYPE_LOG:
			m.logViewer.AppendLine(e.Message)

		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED:
			m.updateStepStatus(e.StepName, "completed")

		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED:
			m.updateStepStatus(e.StepName, "failed")
			m.logViewer.AppendLine(fmt.Sprintf("FAILED: %s", e.Message))

		case pb.BuildEventType_BUILD_EVENT_TYPE_DOWNLOAD_PROGRESS:
			m.currentCmd = fmt.Sprintf("Downloading: %.0f%%", e.Progress*100)

		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED:
			if len(m.images) > 0 {
				last := &m.images[len(m.images)-1]
				last.status = "completed"
				last.collapsed = true
			}
			m.succeeded++
			m.currentStep = ""
			m.currentCmd = ""

		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
			if len(m.images) > 0 {
				last := &m.images[len(m.images)-1]
				last.status = "failed"
				last.collapsed = true
			}
			m.failed++
			m.logViewer.AppendLine(fmt.Sprintf("BUILD FAILED: %s", e.Message))
			m.currentStep = ""
			m.currentCmd = ""
		}

		// Always keep listening — channel close triggers BuildDoneMsg
		if m.events != nil {
			return m, listenForEvent(m.events)
		}
		return m, nil

	case BuildDoneMsg:
		m.Done = true
		m.Backgrounded = false
		m.logViewer.Clear()
		return m, nil

	case spinner.TickMsg:
		if !m.Done {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		m.logViewer.Update(msg)
	}
	return m, nil
}

func (m *BuildProgressModel) updateStepStatus(name, status string) {
	if len(m.images) == 0 {
		return
	}
	img := &m.images[len(m.images)-1]
	for i := range img.steps {
		if img.steps[i].name == name {
			img.steps[i].status = status
			break
		}
	}
}

// View renders the build progress.
func (m BuildProgressModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 3

	// Left panel: step tree
	var left strings.Builder
	left.WriteString(styles.TitleStyle.Render(" Steps"))
	left.WriteString("\n")
	if len(m.images) == 0 {
		left.WriteString("\n")
		fmt.Fprintf(&left, "  %s Waiting for build to start...", m.spinner.View())
	}
	for _, img := range m.images {
		fmt.Fprintf(&left, "\n  %s %s", styles.StatusTag(img.status), img.name)
		if img.collapsed {
			left.WriteString("\n")
			continue
		}
		left.WriteString("\n")
		for _, step := range img.steps {
			fmt.Fprintf(&left, "    %s %s\n", styles.StatusTag(step.status), step.name)
		}
	}

	// Progress counter
	total := len(m.images)
	done := m.succeeded + m.failed
	if total > 0 && !m.Done {
		fmt.Fprintf(&left, "\n  %s\n", styles.MutedStyle.Render(fmt.Sprintf("  %d/%d complete", done, total)))
	}

	leftPanel := lipgloss.NewStyle().Width(leftWidth).Render(left.String())

	// Right panel: current step + log
	var right strings.Builder
	right.WriteString(styles.TitleStyle.Render(" Current Step"))
	right.WriteString("\n")
	if m.currentStep != "" {
		fmt.Fprintf(&right, "  %s\n", m.currentStep)
	} else if m.Done {
		right.WriteString(styles.MutedStyle.Render("  All builds finished\n"))
	} else {
		right.WriteString(styles.MutedStyle.Render("  —\n"))
	}
	if m.currentCmd != "" {
		fmt.Fprintf(&right, "  %s\n", styles.WarningStyle.Render(m.currentCmd))
	}
	right.WriteString("\n")

	logView := m.logViewer.View()

	rightPanel := lipgloss.NewStyle().Width(rightWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left, right.String(), logView),
	)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)

	if m.Done {
		summary := fmt.Sprintf("  %d succeeded, %d failed.", m.succeeded, m.failed)
		if m.failed == 0 {
			summary = styles.SuccessStyle.Render(summary)
		} else {
			summary = styles.ErrorStyle.Render(summary)
		}
		content += "\n\n" + summary + "  " + styles.MutedStyle.Render("Press esc to go back.")
	} else {
		content += "\n\n" + styles.MutedStyle.Render("  esc: send to background")
	}

	return content
}
