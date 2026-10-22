package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/views"
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
	session       *buildSession
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
		return m.buildProgress.InSubView()
	}
	return false
}

// Update implements tea.Model.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViews()
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits
		if msg.String() == "ctrl+c" {
			if m.session != nil {
				m.session.close()
			}
			return m, tea.Quit
		}

		if m.width < 40 || m.height < 12 {
			return m, nil
		}
		if !m.inSubView() && (msg.String() == "ctrl+g" || msg.String() == "f2") {
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
				if m.session != nil {
					m.session.close()
				}
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
	case views.BuildStreamMsg, views.BuildEventMsg, views.BuildDoneMsg, views.BuildCancelResultMsg:
		var cmd tea.Cmd
		m.buildProgress, cmd = m.buildProgress.Update(msg)
		// If we're on the build progress view, return the cmd to keep listening
		if m.currentView == ViewBuildProgress {
			return m, cmd
		}
		// Backgrounded — still process events but don't show them
		return m, cmd

	case spinner.TickMsg:
		if m.buildProgress.Running() {
			var cmd tea.Cmd
			m.buildProgress, cmd = m.buildProgress.Update(msg)
			if m.currentView == ViewVMLaunch {
				var vmCmd tea.Cmd
				m.vmLaunch, vmCmd = m.vmLaunch.Update(msg)
				return m, tea.Batch(cmd, vmCmd)
			}
			return m, cmd
		}
	case views.ReconnectBuildMsg:
		if m.session != nil && msg.ID != "" && msg.ID == m.buildProgress.ActiveBuildID() {
			m.session.reconnect()
		}
		return m, nil
	case views.CancelBuildMsg:
		if m.session != nil && msg.ID != "" && msg.ID == m.buildProgress.ActiveBuildID() {
			return m, m.session.cancelBuild(msg.ID)
		}
		return m, nil

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
		if len(msg.Requests) == 0 {
			return m, nil
		}
		m.session = newBuildSession(msg.Node)
		m.buildProgress.Reset(m.session.events)
		m.currentView = ViewBuildProgress
		go m.session.run(m.store, msg.Requests)
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
	case ViewTemplates:
		return styles.HelpBar(styles.HelpEntry("b", "build"), styles.HelpEntry("/", "filter"), styles.HelpEntry("q", "quit"))
	case ViewBuildProgress:
		return styles.HelpBar(styles.HelpEntry("j/k", "logs"), styles.HelpEntry("f", "follow"), styles.HelpEntry("esc", "templates"))
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

// Run starts the BubbleTea program.
func Run(db *store.DB, version string) error {
	p := tea.NewProgram(NewApp(db, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
