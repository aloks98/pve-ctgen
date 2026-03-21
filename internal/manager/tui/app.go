package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

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
	return AppModel{
		currentView:   ViewHome,
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
		contentHeight := msg.Height - 4 // title + statusbar
		m.cloudinitView.SetSize(msg.Width, contentHeight)
		m.templatesView.SetSize(msg.Width, contentHeight)
		m.stepsView.SetSize(msg.Width, contentHeight)
		m.buildsView.SetSize(msg.Width, contentHeight)
		m.buildSelect.SetSize(msg.Width, contentHeight)
		m.buildProgress.SetSize(msg.Width, contentHeight)
		m.vmLaunch.SetSize(msg.Width, contentHeight)
		m.nodesView.SetSize(msg.Width, contentHeight)
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// If a sub-view has a form/modal open, delegate everything to it
		if m.inSubView() {
			break // fall through to delegate
		}

		switch msg.String() {
		case "q":
			if m.currentView == ViewHome {
				return m, tea.Quit
			}
			if m.currentView == ViewBuildProgress && m.buildProgress.Running() {
				m.buildProgress.Backgrounded = true
			}
			m.currentView = ViewHome
			m.homeModel.Refresh()
			return m, nil
		case "esc":
			if m.currentView == ViewBuildProgress && m.buildProgress.Running() {
				m.buildProgress.Backgrounded = true
			}
			if m.currentView != ViewHome {
				m.currentView = ViewHome
				m.homeModel.Refresh()
				return m, nil
			}
			return m, tea.Quit
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

	case views.StartBuildMsg:
		// Template + node selected — launch the build
		events := make(chan *pb.BuildEvent, 100)
		m.buildProgress.Reset(events)
		m.currentView = ViewBuildProgress
		go runBuildInBackground(m.store, msg.Templates, msg.Node, events)
		return m, m.buildProgress.Init()

	case NavigateMsg:
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

	title := styles.TitleStyle.Width(m.width).Render(" pvectgen")

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

	// Pad content to fill available height
	contentHeight := m.height - 3 // title(1) + statusbar(1) + padding
	contentLines := strings.Count(content, "\n")
	if contentLines < contentHeight {
		content += strings.Repeat("\n", contentHeight-contentLines)
	}

	m.statusBar.Width = m.width
	m.statusBar.Breadcrumb = m.breadcrumb()
	m.statusBar.HelpKeys = m.helpKeys()
	m.statusBar.RightText = m.rightStatus()
	statusBar := m.statusBar.View()

	return lipgloss.JoinVertical(lipgloss.Left, title, content, statusBar)
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
			styles.HelpEntry("q", "home"),
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
func runBuildInBackground(db *store.DB, templates []models.Template, node models.Node, events chan<- *pb.BuildEvent) {
	defer close(events)

	// Gather build steps
	steps, _ := db.ListBuildSteps()
	var pbSteps []*pb.BuildStep
	for _, s := range steps {
		pbSteps = append(pbSteps, &pb.BuildStep{Name: s.Name, Command: s.Command})
	}

	client, err := managergrpc.NewClient(node.Address, node.APIKey)
	if err != nil {
		events <- &pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
			Message: fmt.Sprintf("connect to %s: %v", node.Label(), err),
		}
		return
	}
	defer client.Close()

	for _, tmpl := range templates {
		buildID := uuid.New().String()

		// Resolve cloud-init content
		var ciContent []byte
		var ciFilename string
		if tmpl.CloudInit != "" {
			ci, err := db.GetCloudInitByID(tmpl.CloudInit)
			if err == nil {
				ciContent = []byte(ci.Content)
				ciFilename = ci.Name
			}
		}

		db.CreateBuild(buildID, tmpl.Name, node.Name)

		req := &pb.BuildRequest{
			BuildId: buildID,
			Image: &pb.Image{
				Id:          int32(tmpl.VMID),
				Name:        tmpl.Name,
				Url:         tmpl.URL,
				ChecksumUrl: tmpl.ChecksumURL,
				Tags:        tmpl.Tags,
				Vendor:      ciFilename,
			},
			Steps:             pbSteps,
			CloudinitContent:  ciContent,
			CloudinitFilename: ciFilename,
		}

		stream, err := client.Build(context.Background(), req)
		if err != nil {
			events <- &pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				BuildId: buildID,
				Message: fmt.Sprintf("RPC failed for %s: %v", tmpl.Name, err),
			}
			db.UpdateBuildStatus(buildID, "failed")
			continue
		}

		var failed bool
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				events <- &pb.BuildEvent{
					Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
					BuildId: buildID,
					Message: fmt.Sprintf("stream error: %v", err),
				}
				failed = true
				break
			}
			events <- event
			if event.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED {
				break
			}
			if event.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED {
				failed = true
				break
			}
		}

		if failed {
			db.UpdateBuildStatus(buildID, "failed")
		} else {
			db.UpdateBuildStatus(buildID, "completed")
		}
	}
}

// Run starts the BubbleTea program.
func Run(db *store.DB, version string) error {
	p := tea.NewProgram(NewApp(db, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
