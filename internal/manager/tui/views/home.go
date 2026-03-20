package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

type menuItem struct {
	label string
	desc  string
	view  int // maps to tui.View constants
}

var menuItems = []menuItem{
	{"Nodes", "Manage Proxmox nodes (Minions)", 8},
	{"Cloud-Init Store", "Manage cloud-init configurations", 1},
	{"Template Store", "Manage VM template definitions", 2},
	{"Build Steps", "Manage build step commands", 3},
	{"Build History", "View past build results", 4},
	{"New Build", "Select templates and trigger a build", 5},
	{"Launch VM", "Clone a template and start a VM", 7},
}

// HomeModel is the main menu.
type HomeModel struct {
	cursor    int
	navTarget int
	hasNav    bool
}

// NewHomeModel creates a new HomeModel.
func NewHomeModel() HomeModel {
	return HomeModel{}
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
		switch km.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter":
			m.navTarget = menuItems[m.cursor].view
			m.hasNav = true
		}
	}
	return m, nil
}

// View renders the home menu.
func (m HomeModel) View() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(styles.MutedStyle.Render("  Proxmox VE Cloud-Init Template Generator"))
	b.WriteString("\n\n")

	for i, item := range menuItems {
		cursor := "  "
		if i == m.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}

		label := item.label
		if i == m.cursor {
			label = styles.SelectedStyle.Render(label)
		}

		b.WriteString(fmt.Sprintf("%s%-20s  %s\n", cursor, label, styles.MutedStyle.Render(item.desc)))
	}

	return b.String()
}
