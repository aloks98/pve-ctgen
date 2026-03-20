package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// StartBuildMsg is emitted when the user confirms template + node selection.
type StartBuildMsg struct {
	Templates []models.Template
	Node      models.Node
}

// BuildSelectModel handles template and node selection before a build.
type BuildSelectModel struct {
	db        *store.DB
	templates []selTemplate
	nodes     []models.Node
	cursor    int
	phase     int // 0=templates, 1=nodes
	statusMsg string
	width     int
	height    int
}

type selTemplate struct {
	tmpl     models.Template
	selected bool
}

// NewBuildSelectModel creates a new BuildSelectModel.
func NewBuildSelectModel(db *store.DB) BuildSelectModel {
	return BuildSelectModel{db: db}
}

// SetSize sets the view dimensions.
func (m *BuildSelectModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Refresh reloads data from the store.
func (m *BuildSelectModel) Refresh() {
	m.phase = 0
	m.cursor = 0
	m.statusMsg = ""

	templates, _ := m.db.ListTemplates()
	m.templates = make([]selTemplate, len(templates))
	for i, t := range templates {
		m.templates[i] = selTemplate{tmpl: t}
	}

	m.nodes, _ = m.db.ListNodes()
}

// InSubView returns true so esc/q don't navigate away while selecting.
func (m *BuildSelectModel) InSubView() bool {
	return m.phase == 1
}

// Update handles key events.
func (m BuildSelectModel) Update(msg tea.Msg) (BuildSelectModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.maxIndex() {
				m.cursor++
			}
		case " ":
			if m.phase == 0 && m.cursor < len(m.templates) {
				m.templates[m.cursor].selected = !m.templates[m.cursor].selected
			}
		case "a":
			if m.phase == 0 {
				// Select all / deselect all
				allSelected := true
				for _, t := range m.templates {
					if !t.selected {
						allSelected = false
						break
					}
				}
				for i := range m.templates {
					m.templates[i].selected = !allSelected
				}
			}
		case "enter":
			if m.phase == 0 {
				// Check at least one template selected
				hasSelected := false
				for _, t := range m.templates {
					if t.selected {
						hasSelected = true
						break
					}
				}
				if !hasSelected {
					m.statusMsg = "Select at least one template (space to toggle, a to toggle all)"
					return m, nil
				}
				m.phase = 1
				m.cursor = 0
				m.statusMsg = ""
			} else if m.phase == 1 {
				// Node selected — emit StartBuildMsg
				if len(m.nodes) == 0 {
					m.statusMsg = "No nodes configured"
					return m, nil
				}
				var selected []models.Template
				for _, t := range m.templates {
					if t.selected {
						selected = append(selected, t.tmpl)
					}
				}
				node := m.nodes[m.cursor]
				return m, func() tea.Msg {
					return StartBuildMsg{Templates: selected, Node: node}
				}
			}
		case "esc":
			if m.phase == 1 {
				m.phase = 0
				m.cursor = 0
				m.statusMsg = ""
				return m, nil
			}
		case "tab":
			if m.phase == 0 {
				m.phase = 1
				m.cursor = 0
			} else {
				m.phase = 0
				m.cursor = 0
			}
			m.statusMsg = ""
		}
	}
	return m, nil
}

func (m BuildSelectModel) maxIndex() int {
	if m.phase == 0 {
		return max(0, len(m.templates)-1)
	}
	return max(0, len(m.nodes)-1)
}

// View renders the build selection screen.
func (m BuildSelectModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" New Build "))
	b.WriteString("\n\n")

	// Templates section
	phase := ""
	if m.phase == 0 {
		phase = styles.SelectedStyle.Render(" (active)")
	}
	fmt.Fprintf(&b, "  Templates%s\n\n", phase)
	if len(m.templates) == 0 {
		b.WriteString(styles.MutedStyle.Render("    No templates. Add some first."))
		b.WriteString("\n")
	}
	for i, t := range m.templates {
		cursor := "  "
		if m.phase == 0 && i == m.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}
		check := "[ ]"
		if t.selected {
			check = styles.SuccessStyle.Render("[x]")
		}
		name := t.tmpl.Name
		if m.phase == 0 && i == m.cursor {
			name = styles.SelectedStyle.Render(name)
		}
		fmt.Fprintf(&b, "  %s%s %s (VM %d)\n", cursor, check, name, t.tmpl.VMID)
	}

	b.WriteString("\n")

	// Nodes section
	phase = ""
	if m.phase == 1 {
		phase = styles.SelectedStyle.Render(" (active)")
	}
	fmt.Fprintf(&b, "  Target Node%s\n\n", phase)
	if len(m.nodes) == 0 {
		b.WriteString(styles.MutedStyle.Render("    No nodes configured. Add one via Nodes menu first."))
		b.WriteString("\n")
	}
	for i, n := range m.nodes {
		cursor := "  "
		if m.phase == 1 && i == m.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}
		label := n.Label()
		if m.phase == 1 && i == m.cursor {
			label = styles.SelectedStyle.Render(label)
		}
		extra := ""
		if n.DisplayName != "" && n.DisplayName != n.Name {
			extra = styles.MutedStyle.Render(fmt.Sprintf(" (%s)", n.Name))
		}
		fmt.Fprintf(&b, "  %s%s%s  %s\n", cursor, label, extra, styles.MutedStyle.Render(n.Address))
	}

	b.WriteString("\n")
	if m.phase == 0 {
		b.WriteString(styles.MutedStyle.Render("  space: toggle • a: toggle all • enter: confirm → pick node • tab: switch"))
	} else {
		b.WriteString(styles.MutedStyle.Render("  enter: start build • esc: back to templates • tab: switch"))
	}

	if m.statusMsg != "" {
		b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
	}

	return b.String()
}
