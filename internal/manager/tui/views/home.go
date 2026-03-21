package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

const logo = `▗▄▄▖ ▗▖  ▗▖▗▄▄▄▖     ▗▄▄▖▗▄▄▄▖▗▄▄▖▗▄▄▄▖▗▖  ▗▖
▐▌ ▐▌▐▌  ▐▌▐▌       ▐▌     █ ▐▌   ▐▌   ▐▛▚▖▐▌
▐▛▀▘ ▐▌  ▐▌▐▛▀▀▘    ▐▌     █ ▐▌▝▜▌▐▛▀▀▘▐▌ ▝▜▌
▐▌    ▝▚▞▘ ▐▙▄▄▖    ▝▚▄▄▖  █ ▝▚▄▞▘▐▙▄▄▖▐▌  ▐▌`

type menuItem struct {
	key   string
	label string
	desc  string
	view  int
}

var menuItems = []menuItem{
	{"1", "Nodes", "Manage Proxmox nodes", 8},
	{"2", "Cloud-Init", "Cloud-init configurations", 1},
	{"3", "Templates", "VM template definitions", 2},
	{"4", "Steps", "Build step commands", 3},
	{"5", "History", "Past build results", 4},
	{"6", "Build", "Start a new build", 5},
	{"7", "Launch VM", "Clone and launch a VM", 7},
}

const viewBuildProgressID = 6

// HomeModel is the main dashboard.
type HomeModel struct {
	cursor        int
	navTarget     int
	hasNav        bool
	BuildRunning  bool
	Version       string
	db            *store.DB
	nodeCount     int
	templateCount int
	buildCount    int
}

// NewHomeModel creates a new HomeModel.
func NewHomeModel(db *store.DB, version string) HomeModel {
	return HomeModel{db: db, Version: version}
}

// Refresh loads stats from DB.
func (m *HomeModel) Refresh() {
	if m.db == nil {
		return
	}
	nodes, _ := m.db.ListNodes()
	m.nodeCount = len(nodes)
	templates, _ := m.db.ListTemplates()
	m.templateCount = len(templates)
	builds, _ := m.db.ListBuilds("", "", "", 0)
	m.buildCount = len(builds)
}

// Navigate returns the target view if navigation was triggered.
func (m *HomeModel) Navigate() (int, bool) {
	return m.navTarget, m.hasNav
}

// ResetNavigate clears the navigation flag.
func (m *HomeModel) ResetNavigate() {
	m.hasNav = false
}

// Update handles key events.
func (m HomeModel) Update(msg tea.Msg) (HomeModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		items := m.menuItems()
		switch km.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(items) {
				m.navTarget = items[m.cursor].view
				m.hasNav = true
			}
		case "1", "2", "3", "4", "5", "6", "7":
			idx := int(km.String()[0] - '1')
			offset := 0
			if m.BuildRunning {
				offset = 1
			}
			actual := idx - offset
			if actual >= 0 && actual < len(menuItems) {
				m.navTarget = menuItems[actual].view
				m.hasNav = true
			}
		}
	}
	return m, nil
}

func (m HomeModel) menuItems() []menuItem {
	if m.BuildRunning {
		return append([]menuItem{{"0", "Build Progress", "View running build", viewBuildProgressID}}, menuItems...)
	}
	return menuItems
}

// View renders the home dashboard.
func (m HomeModel) View() string {
	var b strings.Builder

	// ASCII art header
	logoStyle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	b.WriteString(logoStyle.Render(logo))
	b.WriteString("  ")
	b.WriteString(styles.DimStyle.Render(m.Version))
	b.WriteString("\n")
	b.WriteString(styles.DimStyle.Render("  Proxmox VE Cloud-Init Template Generator"))
	b.WriteString("\n\n")

	// Stats row
	stat := func(label string, count int) string {
		return styles.AccentStyle.Render(label) + " " + styles.BoldStyle.Render(fmt.Sprintf("%d", count))
	}
	b.WriteString(fmt.Sprintf(" %s    %s    %s",
		stat("nodes", m.nodeCount),
		stat("templates", m.templateCount),
		stat("builds", m.buildCount),
	))
	b.WriteString("\n\n")

	// Build running indicator
	if m.BuildRunning {
		b.WriteString(" " + styles.WarningStyle.Render("[..] Build running in background") + "\n\n")
	}

	// Menu
	items := m.menuItems()
	for i, item := range items {
		cursor := " "
		if i == m.cursor {
			cursor = styles.SelectedStyle.Render(">")
		}

		key := styles.DimStyle.Render("[" + item.key + "]")
		label := styles.MenuItemStyle.Render(item.label)
		if i == m.cursor {
			label = styles.SelectedStyle.Render(item.label)
		}
		desc := styles.MenuDescStyle.Render(item.desc)

		fmt.Fprintf(&b, " %s %s %-16s %s\n", cursor, key, label, desc)
	}

	return b.String()
}
