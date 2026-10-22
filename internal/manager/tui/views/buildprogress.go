package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type BuildEventMsg struct{ Event *pb.BuildEvent }
type BuildDoneMsg struct{}
type BuildStreamMsg struct{ Message tea.Msg }
type BuildStartedMsg struct {
	Request *pb.BuildRequest
	Node    string
}
type BuildConnectionMsg struct{ Err error }
type BuildConnectedMsg struct{}
type CancelBuildMsg struct{ ID string }
type ReconnectBuildMsg struct{ ID string }
type BuildCancelResultMsg struct {
	ID  string
	Err error
}
type stepStatus struct{ name, status string }

type BuildProgressModel struct {
	logViewer                        components.LogViewer
	events                           <-chan tea.Msg
	spinner                          spinner.Model
	steps                            []stepStatus
	id, name, node                   string
	vmID                             int32
	started                          time.Time
	elapsed                          time.Duration
	lastSequence                     int64
	currentStep, currentCmd          string
	Done, Backgrounded, Disconnected bool
	confirmCancel, cancelling        bool
	statusMsg                        string
	succeeded, failed, total         int
	width, height                    int
}

func NewBuildProgressModel() BuildProgressModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{Frames: []string{"-", "\\", "|", "/"}, FPS: time.Second / 10}
	return BuildProgressModel{logViewer: components.NewLogViewer("Build output"), spinner: s}
}
func (m *BuildProgressModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	logWidth := w
	if w >= 75 {
		logWidth = w - 28
	}
	m.logViewer.SetSize(max(1, logWidth), max(1, h-9))
}
func (m *BuildProgressModel) Reset(events <-chan tea.Msg) {
	width, height := m.width, m.height
	*m = NewBuildProgressModel()
	m.events = events
	m.SetSize(width, height)
}
func (m *BuildProgressModel) Running() bool { return m.events != nil && !m.Done }
func (m *BuildProgressModel) ActiveBuildID() string {
	if m.Running() {
		return m.id
	}
	return ""
}
func (m *BuildProgressModel) InSubView() bool { return m.confirmCancel }
func (m *BuildProgressModel) Init() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return tea.Batch(listenForEvent(m.events), m.spinner.Tick)
}
func listenForEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return BuildDoneMsg{}
		}
		return BuildStreamMsg{Message: msg}
	}
}
func (m BuildProgressModel) Update(msg tea.Msg) (BuildProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case BuildStreamMsg:
		m, _ = m.Update(msg.Message)
		return m, listenForEvent(m.events)
	case BuildStartedMsg:
		req := msg.Request
		m.id = req.BuildId
		m.name = req.Image.Name
		m.vmID = req.Image.Id
		m.node = msg.Node
		m.started = time.Now()
		m.total++
		m.currentStep = "Starting"
		m.currentCmd = ""
		m.lastSequence = 0
		m.Disconnected = false
		m.cancelling = false
		m.confirmCancel = false
		m.statusMsg = ""
		m.steps = []stepStatus{{"Download & Verify", "pending"}, {"Copy Image", "pending"}}
		for _, s := range req.Steps {
			m.steps = append(m.steps, stepStatus{s.Name, "pending"})
		}
		m.logViewer.AppendLine(fmt.Sprintf("--- %s / VMID %d / %s ---", m.name, m.vmID, m.id))
	case BuildConnectedMsg:
		m.Disconnected = false
		m.statusMsg = ""
	case BuildConnectionMsg:
		m.Disconnected = true
		m.statusMsg = msg.Err.Error()
		m.logViewer.AppendLine("Connection interrupted: " + msg.Err.Error())
	case BuildCancelResultMsg:
		if msg.ID != m.id || m.Done {
			return m, nil
		}
		if msg.Err != nil {
			m.cancelling = false
			m.statusMsg = "Cancel failed: " + msg.Err.Error()
		} else {
			m.statusMsg = "Cancellation requested; waiting for minion confirmation."
		}
	case BuildEventMsg:
		e := msg.Event
		if e == nil {
			return m, nil
		}
		if m.id != "" && e.BuildId != "" && e.BuildId != m.id {
			return m, nil
		}
		if e.Sequence > 0 && e.Sequence <= m.lastSequence {
			return m, nil
		}
		if e.Sequence > 0 {
			m.lastSequence = e.Sequence
		}
		switch e.Type {
		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED:
			m.currentStep = e.StepName
			m.currentCmd = ""
			m.updateStep(e.StepName, "running")
		case pb.BuildEventType_BUILD_EVENT_TYPE_LOG:
			m.logViewer.AppendLine(e.Message)
		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED:
			m.updateStep(e.StepName, "completed")
		case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED:
			m.updateStep(e.StepName, "failed")
			m.logViewer.AppendLine("FAILED: " + e.Message)
		case pb.BuildEventType_BUILD_EVENT_TYPE_DOWNLOAD_PROGRESS:
			m.currentCmd = fmt.Sprintf("Downloading %.0f%%", e.Progress*100)
		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED:
			m.succeeded++
			m.currentStep = "Completed"
			m.currentCmd = ""
			m.logViewer.AppendLine("BUILD COMPLETED")
		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
			m.failed++
			m.currentStep = "Failed"
			m.currentCmd = ""
			m.logViewer.AppendLine("BUILD FAILED: " + e.Message)
		}
	case BuildDoneMsg:
		m.Done = true
		m.Backgrounded = false
		m.Disconnected = false
		m.confirmCancel = false
		m.cancelling = false
		m.statusMsg = ""
		if !m.started.IsZero() {
			m.elapsed = time.Since(m.started)
		}
	case spinner.TickMsg:
		if m.Running() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case tea.KeyMsg:
		k := msg.String()
		if m.confirmCancel {
			switch k {
			case "y":
				m.confirmCancel = false
				m.cancelling = true
				id := m.id
				return m, func() tea.Msg { return CancelBuildMsg{ID: id} }
			case "n", "esc":
				m.confirmCancel = false
			}
			return m, nil
		}
		if k == "x" && m.Running() && m.id != "" && !m.cancelling {
			m.confirmCancel = true
			return m, nil
		}
		if k == "r" && m.Disconnected {
			m.statusMsg = "Reconnecting..."
			id := m.id
			return m, func() tea.Msg { return ReconnectBuildMsg{ID: id} }
		}
		m.logViewer.Update(msg)
	}
	return m, nil
}
func (m *BuildProgressModel) updateStep(name, status string) {
	for i := range m.steps {
		if m.steps[i].name == name {
			m.steps[i].status = status
			return
		}
	}
	m.steps = append(m.steps, stepStatus{name, status})
}
func (m BuildProgressModel) View() string {
	if m.events == nil {
		return "Build progress\n\nNo build in this session. Choose a template and press b.\nPrevious results are available in History."
	}
	state := "RUNNING " + m.spinner.View()
	if m.Done {
		state = "FINISHED"
	}
	if m.Disconnected {
		state = "DISCONNECTED"
	}
	if m.cancelling {
		state = "CANCELLING"
	}
	elapsed := m.elapsed
	if !m.Done && !m.started.IsZero() {
		elapsed = time.Since(m.started)
	}
	heading := styles.BoldStyle.Render(m.name) + "  " + styles.WarningStyle.Render(state) + "\n" + styles.DimStyle.Render(fmt.Sprintf("%s / VMID %d / %s / %s", m.node, m.vmID, m.id, elapsed.Round(time.Second))) + "\n\n"
	current := m.currentStep
	if m.currentCmd != "" {
		current += " / " + m.currentCmd
	}
	heading += styles.AccentStyle.Render(current) + "\n"
	body := m.logViewer.View()
	if m.width >= 75 {
		var lines []string
		for _, step := range m.steps {
			lines = append(lines, styles.StatusTag(step.status)+" "+step.name)
		}
		// Keep the active step visible when custom pipelines exceed the terminal height.
		active := 0
		for i, s := range m.steps {
			if s.status == "running" {
				active = i
			}
		}
		visible := max(1, m.height-9)
		start := max(0, active-visible+1)
		left := styles.Fit(strings.Join(lines[start:], "\n"), 26, visible+1)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", body)
	}
	footer := fmt.Sprintf("%d succeeded / %d failed / %d started", m.succeeded, m.failed, m.total)
	if m.confirmCancel {
		footer = "Cancel build and skip queued builds?\ny confirm / n or esc keep running"
	} else if m.Disconnected {
		footer += " | r reconnect | x cancel"
	} else if !m.Done {
		footer += " | x cancel | esc background"
	}
	if m.statusMsg != "" {
		footer += "\n" + m.statusMsg
	}
	return heading + body + "\n" + styles.DimStyle.Render(footer)
}
