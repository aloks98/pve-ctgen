package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// StartBuildMsg is emitted when the user confirms template + node + VM ID selection.
type StartBuildMsg struct {
	Templates []models.Template
	Node      models.Node
}

const (
	phaseTemplates = 0
	phaseNodes     = 1
	phaseVMIDs     = 2 // override VM IDs before starting
)

// BuildSelectModel handles template and node selection before a build.
type BuildSelectModel struct {
	db           *store.DB
	templates    []selTemplate
	nodes        []models.Node
	selectedNode *models.Node
	cursor       int
	phase        int
	statusMsg    string

	// VM ID override inputs (populated when entering phase 2)
	vmIDInputs []vmIDOverride

	width  int
	height int
}

type selTemplate struct {
	tmpl     models.Template
	selected bool
}

type vmIDOverride struct {
	templateName string
	defaultVMID  int
	input        textinput.Model
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
	m.phase = phaseTemplates
	m.cursor = 0
	m.statusMsg = ""
	m.vmIDInputs = nil

	templates, _ := m.db.ListTemplates()
	m.templates = make([]selTemplate, len(templates))
	for i, t := range templates {
		m.templates[i] = selTemplate{tmpl: t}
	}

	m.nodes, _ = m.db.ListNodes()
}

// InSubView returns true so esc/q don't navigate away while in sub-phases.
func (m *BuildSelectModel) InSubView() bool {
	return m.phase > phaseTemplates
}

// Update handles key events.
func (m BuildSelectModel) Update(msg tea.Msg) (BuildSelectModel, tea.Cmd) {
	switch m.phase {
	case phaseVMIDs:
		return m.updateVMIDPhase(msg)
	}

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
			if m.phase == phaseTemplates && m.cursor < len(m.templates) {
				m.templates[m.cursor].selected = !m.templates[m.cursor].selected
			}
		case "a":
			if m.phase == phaseTemplates {
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
			if m.phase == phaseTemplates {
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
				m.phase = phaseNodes
				m.cursor = 0
				m.statusMsg = ""
			} else if m.phase == phaseNodes {
				if len(m.nodes) == 0 {
					m.statusMsg = "No nodes configured"
					return m, nil
				}
				// Store selected node
				node := m.nodes[m.cursor]
				m.selectedNode = &node
				// Build VM ID override inputs for selected templates
				m.vmIDInputs = nil
				for _, t := range m.templates {
					if !t.selected {
						continue
					}
					ti := textinput.New()
					ti.Placeholder = fmt.Sprintf("%d", t.tmpl.VMID)
					ti.SetValue(fmt.Sprintf("%d", t.tmpl.VMID))
					ti.CharLimit = 10
					ti.Width = 8
					m.vmIDInputs = append(m.vmIDInputs, vmIDOverride{
						templateName: t.tmpl.Name,
						defaultVMID:  t.tmpl.VMID,
						input:        ti,
					})
				}
				if len(m.vmIDInputs) > 0 {
					m.vmIDInputs[0].input.Focus()
				}
				m.phase = phaseVMIDs
				m.cursor = 0
				m.statusMsg = ""
				return m, nil
			}
		case "esc":
			if m.phase == phaseNodes {
				m.phase = phaseTemplates
				m.cursor = 0
				m.statusMsg = ""
				return m, nil
			}
		case "tab":
			if m.phase == phaseTemplates {
				m.phase = phaseNodes
				m.cursor = 0
			} else {
				m.phase = phaseTemplates
				m.cursor = 0
			}
			m.statusMsg = ""
		}
	}
	return m, nil
}

func (m BuildSelectModel) updateVMIDPhase(msg tea.Msg) (BuildSelectModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "tab", "down":
			m.vmIDInputs[m.cursor].input.Blur()
			m.cursor = (m.cursor + 1) % len(m.vmIDInputs)
			return m, m.vmIDInputs[m.cursor].input.Focus()
		case "shift+tab", "up":
			m.vmIDInputs[m.cursor].input.Blur()
			m.cursor = (m.cursor - 1 + len(m.vmIDInputs)) % len(m.vmIDInputs)
			return m, m.vmIDInputs[m.cursor].input.Focus()
		case "enter":
			// If not on last field, go next
			if m.cursor < len(m.vmIDInputs)-1 {
				m.vmIDInputs[m.cursor].input.Blur()
				m.cursor++
				return m, m.vmIDInputs[m.cursor].input.Focus()
			}
			// Last field — validate and start build
			return m.submitBuild()
		case "ctrl+s":
			return m.submitBuild()
		case "esc":
			m.phase = phaseNodes
			m.cursor = 0
			m.statusMsg = ""
			return m, nil
		}
	}

	// Update focused input
	var cmd tea.Cmd
	m.vmIDInputs[m.cursor].input, cmd = m.vmIDInputs[m.cursor].input.Update(msg)
	return m, cmd
}

func (m BuildSelectModel) submitBuild() (BuildSelectModel, tea.Cmd) {
	// Validate all VM IDs
	seen := make(map[int]string)
	for i := range m.vmIDInputs {
		val := m.vmIDInputs[i].input.Value()
		if val == "" {
			val = fmt.Sprintf("%d", m.vmIDInputs[i].defaultVMID)
		}
		vmID, err := strconv.Atoi(val)
		if err != nil {
			m.statusMsg = fmt.Sprintf("%s: VM ID must be a number", m.vmIDInputs[i].templateName)
			return m, nil
		}
		if vmID <= 0 {
			m.statusMsg = fmt.Sprintf("%s: VM ID must be positive", m.vmIDInputs[i].templateName)
			return m, nil
		}
		if prev, ok := seen[vmID]; ok {
			m.statusMsg = fmt.Sprintf("Duplicate VM ID %d: %s and %s", vmID, prev, m.vmIDInputs[i].templateName)
			return m, nil
		}
		seen[vmID] = m.vmIDInputs[i].templateName
	}

	// Build the template list with overridden VM IDs
	var selected []models.Template
	idx := 0
	for _, t := range m.templates {
		if !t.selected {
			continue
		}
		tmpl := t.tmpl
		val := m.vmIDInputs[idx].input.Value()
		if val != "" {
			vmID, _ := strconv.Atoi(val)
			tmpl.VMID = vmID
		}
		selected = append(selected, tmpl)
		idx++
	}

	node := *m.selectedNode
	return m, func() tea.Msg {
		return StartBuildMsg{Templates: selected, Node: node}
	}
}

func (m BuildSelectModel) maxIndex() int {
	if m.phase == phaseTemplates {
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

	if m.phase == phaseVMIDs {
		return m.viewVMIDPhase()
	}

	// Templates section
	phase := ""
	if m.phase == phaseTemplates {
		phase = styles.SelectedStyle.Render(" (active)")
	}
	fmt.Fprintf(&b, "  Templates%s\n\n", phase)
	if len(m.templates) == 0 {
		b.WriteString(styles.MutedStyle.Render("    No templates. Add some first.\n"))
	}
	for i, t := range m.templates {
		cursor := "  "
		if m.phase == phaseTemplates && i == m.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}
		check := "[ ]"
		if t.selected {
			check = styles.SuccessStyle.Render("[x]")
		}
		name := t.tmpl.Name
		if m.phase == phaseTemplates && i == m.cursor {
			name = styles.SelectedStyle.Render(name)
		}
		fmt.Fprintf(&b, "  %s%s %s (VM %d)\n", cursor, check, name, t.tmpl.VMID)
	}

	b.WriteString("\n")

	// Nodes section
	phase = ""
	if m.phase == phaseNodes {
		phase = styles.SelectedStyle.Render(" (active)")
	}
	fmt.Fprintf(&b, "  Target Node%s\n\n", phase)
	if len(m.nodes) == 0 {
		b.WriteString(styles.MutedStyle.Render("    No nodes configured. Add one via Nodes menu first.\n"))
	}
	for i, n := range m.nodes {
		cursor := "  "
		if m.phase == phaseNodes && i == m.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}
		label := n.Label()
		if m.phase == phaseNodes && i == m.cursor {
			label = styles.SelectedStyle.Render(label)
		}
		extra := ""
		if n.DisplayName != "" && n.DisplayName != n.Name {
			extra = styles.MutedStyle.Render(fmt.Sprintf(" (%s)", n.Name))
		}
		fmt.Fprintf(&b, "  %s%s%s  %s\n", cursor, label, extra, styles.MutedStyle.Render(n.Address))
	}

	b.WriteString("\n")
	if m.phase == phaseTemplates {
		b.WriteString(styles.MutedStyle.Render("  space: toggle • a: toggle all • enter: confirm → pick node • tab: switch"))
	} else {
		b.WriteString(styles.MutedStyle.Render("  enter: configure VM IDs → start • esc: back • tab: switch"))
	}

	if m.statusMsg != "" {
		b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
	}

	return b.String()
}

func (m BuildSelectModel) viewVMIDPhase() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" New Build — Configure VM IDs "))
	b.WriteString("\n\n")
	b.WriteString(styles.MutedStyle.Render("  Override VM IDs or keep defaults:"))
	b.WriteString("\n\n")

	// Find max template name length for alignment
	maxLen := 0
	for _, o := range m.vmIDInputs {
		if len(o.templateName) > maxLen {
			maxLen = len(o.templateName)
		}
	}

	for i, o := range m.vmIDInputs {
		if i == m.cursor {
			padded := fmt.Sprintf("%-*s", maxLen, o.templateName)
			fmt.Fprintf(&b, "  %s %s  VM ID: %s\n",
				styles.SelectedStyle.Render(">"),
				styles.SelectedStyle.Render(padded),
				o.input.View())
		} else {
			fmt.Fprintf(&b, "    %-*s  VM ID: %s\n", maxLen, o.templateName, o.input.View())
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.MutedStyle.Render("  tab/↑↓: navigate • enter: start build • esc: back"))

	if m.statusMsg != "" {
		b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
	}

	return b.String()
}
