package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/buildrun"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/views"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// View represents which screen is active.
type View int

const (
	ViewHome View = iota
	ViewCloudInit
	ViewTemplates
	ViewSteps
	ViewBuilds
	ViewBuildSelect
	ViewBuildProgress
	ViewVMLaunch
	ViewNodes
)

// NavigateMsg tells the app to switch views.
type NavigateMsg struct {
	View View
}

// AppModel is the root BubbleTea model.
type AppModel struct {
	currentView   View
	navFocused    bool
	navCursor     int
	homeModel     views.HomeModel
	cloudinitView views.CloudInitModel
	templatesView views.TemplatesModel
	stepsView     views.StepsModel
	buildsView    views.BuildsModel
	buildSelect   views.BuildSelectModel
	buildProgress views.BuildProgressModel
	vmLaunch      views.VMLaunchModel
	nodesView     views.NodesModel
	statusBar     components.StatusBar
	store         *store.DB
	width, height int
	err           error
}

// NewApp creates a new AppModel.
func NewApp(db *store.DB, version string) AppModel {
	home := views.NewHomeModel(db, version)
	home.Refresh()
	m := AppModel{
		currentView:   ViewTemplates,
		homeModel:     home,
		cloudinitView: views.NewCloudInitModel(db),
		templatesView: views.NewTemplatesModel(db),
		stepsView:     views.NewStepsModel(db),
		buildsView:    views.NewBuildsModel(db),
		buildSelect:   views.NewBuildSelectModel(db),
		buildProgress: views.NewBuildProgressModel(),
		vmLaunch:      views.NewVMLaunchModel(db),
		nodesView:     views.NewNodesModel(db),
		store:         db,
	}
	m.templatesView.Refresh()
	m.nodesView.Refresh()
	return m
}

// Init implements tea.Model.
func (m AppModel) Init() tea.Cmd {
	m.homeModel.Refresh()
	return nil
}

// inSubView returns true if the current view has a modal/form open that should capture esc/q.
func (m *AppModel) inSubView() bool {
	switch m.currentView {
	case ViewCloudInit:
		return m.cloudinitView.InSubView()
	case ViewTemplates:
		return m.templatesView.InSubView()
	case ViewSteps:
		return m.stepsView.InSubView()
	case ViewBuilds:
		return m.buildsView.InSubView()
	case ViewVMLaunch:
		return m.vmLaunch.InSubView()
	case ViewNodes:
		return m.nodesView.InSubView()
	case ViewBuildSelect:
		return m.buildSelect.InSubView()
	case ViewBuildProgress:
		return false // esc sends build to background, never blocks
	}
	return false
}

// Update implements tea.Model.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.Width = msg.Width
		m.resizeViews()
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.width < 40 || m.height < 12 {
			return m, nil
		}
		if !m.inSubView() && msg.String() == "f2" {
			m.navFocused = !m.navFocused
			for i, d := range destinations {
				if d.view == m.currentView {
					m.navCursor = i
				}
			}
			return m, nil
		}
		if m.navFocused {
			switch msg.String() {
			case "up", "k":
				m.navCursor = max(0, m.navCursor-1)
			case "down", "j":
				m.navCursor = min(len(destinations)-1, m.navCursor+1)
			case "esc":
				m.navFocused = false
			case "enter":
				m.navFocused = false
				return m.Update(NavigateMsg{View: destinations[m.navCursor].view})
			default:
				if key := msg.String(); len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
					m.navFocused = false
					return m.Update(NavigateMsg{View: destinations[int(key[0]-'1')].view})
				}
			}
			return m, nil
		}

		// If a sub-view has a form/modal open, delegate everything to it
		if m.inSubView() {
			break // fall through to delegate
		}

		switch msg.String() {
		case "q":
			if m.currentView == ViewTemplates {
				return m, tea.Quit
			}
			if m.currentView == ViewBuildProgress && m.buildProgress.Running() {
				m.buildProgress.Backgrounded = true
			}
			m.currentView = ViewTemplates
			m.templatesView.Refresh()
			return m, nil
		case "esc":
			if m.currentView == ViewBuildProgress && m.buildProgress.Running() {
				m.buildProgress.Backgrounded = true
			}
			if m.currentView != ViewTemplates {
				m.currentView = ViewTemplates
				m.templatesView.Refresh()
				return m, nil
			}
			return m, nil
		}

	// Build events are always handled, even when backgrounded on another view
	case views.BuildEventMsg, views.BuildDoneMsg:
		var cmd tea.Cmd
		m.buildProgress, cmd = m.buildProgress.Update(msg)
		// If we're on the build progress view, return the cmd to keep listening
		if m.currentView == ViewBuildProgress {
			return m, cmd
		}
		// Backgrounded — still process events but don't show them
		return m, cmd

	case views.BuildTemplateMsg:
		if m.buildProgress.Running() {
			m.currentView = ViewBuildProgress
			return m, nil
		}
		m.buildSelect.SelectTemplate(msg.Name)
		m.currentView = ViewBuildSelect
		return m, nil

	case views.StartBuildMsg:
		if m.buildProgress.Running() {
			return m, nil
		}
		// Template + node selected — launch the build
		events := make(chan *pb.BuildEvent, 100)
		m.buildProgress.Reset(events)
		m.currentView = ViewBuildProgress
		go runBuildInBackground(m.store, msg.Templates, msg.Node, msg.Requests, events)
		return m, m.buildProgress.Init()

	case NavigateMsg:
		if msg.View == ViewBuildSelect && m.buildProgress.Running() {
			msg.View = ViewBuildProgress
		}
		m.currentView = msg.View
		switch msg.View {
		case ViewCloudInit:
			m.cloudinitView.Refresh()
		case ViewTemplates:
			m.templatesView.Refresh()
		case ViewSteps:
			m.stepsView.Refresh()
		case ViewBuilds:
			m.buildsView.Refresh()
		case ViewBuildSelect:
			m.buildSelect.Refresh()
		case ViewVMLaunch:
			m.vmLaunch.Refresh()
		case ViewNodes:
			m.nodesView.Refresh()
		}
		return m, nil
	}

	// Sync build status to home
	m.homeModel.BuildRunning = m.buildProgress.Running()

	// Delegate to current view
	var cmd tea.Cmd
	switch m.currentView {
	case ViewHome:
		m.homeModel, cmd = m.homeModel.Update(msg)
		if nav, ok := m.homeModel.Navigate(); ok {
			m.homeModel.ResetNavigate()
			return m.Update(NavigateMsg{View: View(nav)})
		}
	case ViewCloudInit:
		m.cloudinitView, cmd = m.cloudinitView.Update(msg)
	case ViewTemplates:
		m.templatesView, cmd = m.templatesView.Update(msg)
	case ViewSteps:
		m.stepsView, cmd = m.stepsView.Update(msg)
	case ViewBuilds:
		m.buildsView, cmd = m.buildsView.Update(msg)
	case ViewBuildSelect:
		m.buildSelect, cmd = m.buildSelect.Update(msg)
	case ViewBuildProgress:
		m.buildProgress, cmd = m.buildProgress.Update(msg)
	case ViewVMLaunch:
		m.vmLaunch, cmd = m.vmLaunch.Update(msg)
	case ViewNodes:
		m.nodesView, cmd = m.nodesView.Update(msg)
	}

	return m, cmd
}

// View implements tea.Model.
func (m AppModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var content string
	switch m.currentView {
	case ViewHome:
		content = m.homeModel.View()
	case ViewCloudInit:
		content = m.cloudinitView.View()
	case ViewTemplates:
		content = m.templatesView.View()
	case ViewSteps:
		content = m.stepsView.View()
	case ViewBuilds:
		content = m.buildsView.View()
	case ViewBuildSelect:
		content = m.buildSelect.View()
	case ViewBuildProgress:
		content = m.buildProgress.View()
	case ViewVMLaunch:
		content = m.vmLaunch.View()
	case ViewNodes:
		content = m.nodesView.View()
	}

	return m.workspace(content)
}

func (m AppModel) breadcrumb() string {
	base := "pvectgen"
	switch m.currentView {
	case ViewHome:
		return base
	case ViewCloudInit:
		return base + " > cloud-init"
	case ViewTemplates:
		return base + " > templates"
	case ViewSteps:
		return base + " > steps"
	case ViewBuilds:
		return base + " > builds"
	case ViewBuildSelect:
		return base + " > new build"
	case ViewBuildProgress:
		return base + " > build progress"
	case ViewVMLaunch:
		return base + " > launch vm"
	case ViewNodes:
		return base + " > nodes"
	default:
		return base
	}
}

func (m AppModel) helpKeys() string {
	if m.inSubView() {
		return ""
	}
	switch m.currentView {
	case ViewHome:
		return styles.HelpBar(
			styles.HelpEntry("j/k", "move"),
			styles.HelpEntry("enter", "select"),
			styles.HelpEntry("q", "quit"),
		)
	default:
		return styles.HelpBar(
			styles.HelpEntry("j/k", "move"),
			styles.HelpEntry("esc", "back"),
			styles.HelpEntry("q", "templates"),
		)
	}
}

func (m AppModel) rightStatus() string {
	if m.buildProgress.Running() {
		return styles.WarningStyle.Render("[building]")
	}
	return styles.DimStyle.Render(fmt.Sprintf("%d nodes", len(m.nodesView.NodeCount())))
}

// runBuildInBackground connects to the minion, runs builds, and sends events on the channel.
// The channel is closed when all builds complete.
func runBuildInBackground(db *store.DB, templates []models.Template, node models.Node, requests []*pb.BuildRequest, events chan<- *pb.BuildEvent) {
	defer close(events)

	client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
	if err != nil {
		events <- &pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()}
		return
	}
	defer client.Close()
	for i, tmpl := range templates {
		reportedFailure := false
		emit := func(e *pb.BuildEvent) {
			events <- e
			reportedFailure = e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED
		}
		var err error
		if i < len(requests) {
			err = buildrun.Start(context.Background(), db, client, requests[i], node.Name, emit)
		} else {
			err = buildrun.Run(context.Background(), db, client, tmpl, node.Name, nil, emit)
		}
		if err != nil && !reportedFailure {
			events <- &pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()}
		}
	}
}

// Run starts the BubbleTea program.
func Run(db *store.DB, version string) error {
	p := tea.NewProgram(NewApp(db, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
