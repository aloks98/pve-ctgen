package views

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

type vmPhase int

const (
	vmPhaseNodeSelect    vmPhase = iota // Pick a node
	vmPhaseLoadTemplates                // Loading templates
	vmPhaseTmplSelect                   // Pick a template
	vmPhaseConfig                       // Configure VM options
	vmPhaseLaunching                    // Launching
	vmPhaseResult                       // Done
)

type remoteTemplate struct {
	vmID int32
	name string
	tags string
}

type vmLoadedTemplatesMsg struct {
	templates []remoteTemplate
	err       error
}

type vmLaunchResultMsg struct {
	resp *pb.LaunchVMResponse
	err  error
}

// field IDs for the config form
const (
	fldVMID = iota
	fldName
	fldMemory
	fldCores
	fldIPType  // select: dhcp / static
	fldIPAddr  // only visible when static
	fldGateway // only visible when static
	fldStart
	fldStartAtBoot
	fldCount // sentinel
)

type vmField struct {
	label   string
	input   textinput.Model
	options []string // non-empty = select field
	selIdx  int
}

// VMLaunchModel handles the VM launch flow.
type VMLaunchModel struct {
	db    *store.DB
	nodes []models.Node
	phase vmPhase

	nodeCursor      int
	remoteTemplates []remoteTemplate
	tmplCursor      int
	selectedNode    *models.Node
	selectedTmpl    *remoteTemplate

	fields    [fldCount]vmField
	cursor    int
	statusMsg string
	width     int
	height    int
}

// NewVMLaunchModel creates a new VMLaunchModel.
func NewVMLaunchModel(db *store.DB) VMLaunchModel {
	m := VMLaunchModel{db: db}
	m.initFields()
	return m
}

func (m *VMLaunchModel) initFields() {
	mk := func(label, placeholder, def string) vmField {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.SetValue(def)
		ti.CharLimit = 64
		ti.Width = 30
		return vmField{label: label, input: ti}
	}
	m.fields = [fldCount]vmField{
		fldVMID:       mk("VM ID", "e.g. 100", ""),
		fldName:       mk("Name", "e.g. my-webserver", ""),
		fldMemory:     mk("Memory (MB)", "2048", "2048"),
		fldCores:      mk("Cores", "2", "2"),
		fldIPType:     {label: "IP Config", options: []string{"dhcp", "static"}, selIdx: 0},
		fldIPAddr:     mk("IP Address", "e.g. 192.168.1.50/24", ""),
		fldGateway:    mk("Gateway", "e.g. 192.168.1.1", ""),
		fldStart:      {label: "Start after create", options: []string{"yes", "no"}, selIdx: 0},
		fldStartAtBoot: {label: "Start at boot", options: []string{"no", "yes"}, selIdx: 0},
	}
}

// SetSize sets the view dimensions.
func (m *VMLaunchModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Refresh resets to node selection.
func (m *VMLaunchModel) Refresh() {
	m.nodes, _ = m.db.ListNodes()
	m.phase = vmPhaseNodeSelect
	m.nodeCursor = 0
	m.tmplCursor = 0
	m.remoteTemplates = nil
	m.selectedNode = nil
	m.selectedTmpl = nil
	m.statusMsg = ""
	m.cursor = 0
	m.initFields()
}

// InSubView returns true if the view should capture esc/q.
func (m *VMLaunchModel) InSubView() bool {
	return m.phase != vmPhaseNodeSelect
}

func (m *VMLaunchModel) isStaticIP() bool {
	return m.fields[fldIPType].options[m.fields[fldIPType].selIdx] == "static"
}

func (m *VMLaunchModel) visibleFields() []int {
	ids := []int{fldVMID, fldName, fldMemory, fldCores, fldIPType}
	if m.isStaticIP() {
		ids = append(ids, fldIPAddr, fldGateway)
	}
	ids = append(ids, fldStart, fldStartAtBoot)
	return ids
}

// Update handles events.
func (m VMLaunchModel) Update(msg tea.Msg) (VMLaunchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case vmLoadedTemplatesMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			m.phase = vmPhaseNodeSelect
			return m, nil
		}
		m.remoteTemplates = msg.templates
		m.tmplCursor = 0
		if len(msg.templates) == 0 {
			m.statusMsg = "No templates found on this node"
			m.phase = vmPhaseNodeSelect
			return m, nil
		}
		m.phase = vmPhaseTmplSelect
		return m, nil

	case vmLaunchResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Launch failed: %v", msg.err)
		} else if !msg.resp.Success {
			m.statusMsg = fmt.Sprintf("Launch failed: %s", msg.resp.Message)
		} else {
			m.statusMsg = fmt.Sprintf("VM %d created: %s", msg.resp.VmId, msg.resp.Message)
		}
		m.phase = vmPhaseResult
		return m, nil
	}

	switch m.phase {
	case vmPhaseNodeSelect:
		return m.updateNodeSelect(msg)
	case vmPhaseTmplSelect:
		return m.updateTmplSelect(msg)
	case vmPhaseConfig:
		return m.updateConfig(msg)
	case vmPhaseResult:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "enter" {
				m.Refresh()
			}
		}
	}
	return m, nil
}

func (m VMLaunchModel) updateNodeSelect(msg tea.Msg) (VMLaunchModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if m.nodeCursor > 0 {
				m.nodeCursor--
			}
		case "down", "j":
			if m.nodeCursor < len(m.nodes)-1 {
				m.nodeCursor++
			}
		case "enter":
			if len(m.nodes) == 0 {
				return m, nil
			}
			node := m.nodes[m.nodeCursor]
			m.selectedNode = &node
			m.phase = vmPhaseLoadTemplates
			m.statusMsg = ""
			return m, m.fetchTemplatesCmd(node)
		}
	}
	return m, nil
}

func (m VMLaunchModel) updateTmplSelect(msg tea.Msg) (VMLaunchModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if m.tmplCursor > 0 {
				m.tmplCursor--
			}
		case "down", "j":
			if m.tmplCursor < len(m.remoteTemplates)-1 {
				m.tmplCursor++
			}
		case "enter":
			if len(m.remoteTemplates) == 0 {
				return m, nil
			}
			tmpl := m.remoteTemplates[m.tmplCursor]
			m.selectedTmpl = &tmpl
			m.cursor = 0
			m.phase = vmPhaseConfig
			m.statusMsg = ""
			// Focus first input
			m.fields[fldVMID].input.Focus()
			return m, m.fields[fldVMID].input.Focus()
		case "esc":
			m.phase = vmPhaseNodeSelect
		}
	}
	return m, nil
}

func (m VMLaunchModel) updateConfig(msg tea.Msg) (VMLaunchModel, tea.Cmd) {
	visible := m.visibleFields()
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
	fid := visible[m.cursor]
	f := &m.fields[fid]

	if km, ok := msg.(tea.KeyMsg); ok {
		// Select fields
		if len(f.options) > 0 {
			switch km.String() {
			case "left", "h":
				if f.selIdx > 0 {
					f.selIdx--
				}
				return m, nil
			case "right", "l":
				if f.selIdx < len(f.options)-1 {
					f.selIdx++
				}
				return m, nil
			case "tab", "down", "j":
				visible = m.visibleFields() // may have changed after IP type toggle
				m.cursor = (m.cursor + 1) % len(visible)
				m.focusCurrent()
				return m, nil
			case "shift+tab", "up", "k":
				visible = m.visibleFields()
				m.cursor = (m.cursor - 1 + len(visible)) % len(visible)
				m.focusCurrent()
				return m, nil
			case "enter":
				return m.submitConfig()
			case "esc":
				m.phase = vmPhaseTmplSelect
				m.statusMsg = ""
				return m, nil
			}
			return m, nil
		}

		// Text input fields
		switch km.String() {
		case "tab", "down":
			f.input.Blur()
			visible = m.visibleFields()
			m.cursor = (m.cursor + 1) % len(visible)
			m.focusCurrent()
			return m, nil
		case "shift+tab", "up":
			f.input.Blur()
			visible = m.visibleFields()
			m.cursor = (m.cursor - 1 + len(visible)) % len(visible)
			m.focusCurrent()
			return m, nil
		case "enter":
			// If last field, submit; otherwise next
			if m.cursor == len(visible)-1 {
				return m.submitConfig()
			}
			f.input.Blur()
			m.cursor++
			visible = m.visibleFields()
			if m.cursor >= len(visible) {
				m.cursor = len(visible) - 1
			}
			m.focusCurrent()
			return m, nil
		case "esc":
			m.phase = vmPhaseTmplSelect
			m.statusMsg = ""
			return m, nil
		}
	}

	// Update focused text input
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	m.fields[fid] = *f
	return m, cmd
}

func (m *VMLaunchModel) focusCurrent() {
	visible := m.visibleFields()
	for i, fid := range visible {
		if len(m.fields[fid].options) == 0 {
			if i == m.cursor {
				m.fields[fid].input.Focus()
			} else {
				m.fields[fid].input.Blur()
			}
		}
	}
}

func (m VMLaunchModel) submitConfig() (VMLaunchModel, tea.Cmd) {
	vmIDStr := m.fields[fldVMID].input.Value()
	name := m.fields[fldName].input.Value()
	memStr := m.fields[fldMemory].input.Value()
	coresStr := m.fields[fldCores].input.Value()
	start := m.fields[fldStart].options[m.fields[fldStart].selIdx] == "yes"
	startAtBoot := m.fields[fldStartAtBoot].options[m.fields[fldStartAtBoot].selIdx] == "yes"

	if vmIDStr == "" || name == "" {
		m.statusMsg = "VM ID and Name are required"
		return m, nil
	}
	newVMID, err := strconv.Atoi(vmIDStr)
	if err != nil {
		m.statusMsg = "VM ID must be a number"
		return m, nil
	}
	memory, _ := strconv.Atoi(memStr)
	if memory == 0 {
		memory = 2048
	}
	cores, _ := strconv.Atoi(coresStr)
	if cores == 0 {
		cores = 2
	}

	// Build IP config string
	ipConfig := "ip=dhcp"
	if m.isStaticIP() {
		addr := m.fields[fldIPAddr].input.Value()
		gw := m.fields[fldGateway].input.Value()
		if addr == "" {
			m.statusMsg = "IP Address is required for static config (e.g. 192.168.1.50/24)"
			return m, nil
		}
		ipConfig = fmt.Sprintf("ip=%s", addr)
		if gw != "" {
			ipConfig += fmt.Sprintf(",gw=%s", gw)
		}
	}

	m.phase = vmPhaseLaunching
	m.statusMsg = "Launching VM..."
	return m, m.launchVMCmd(m.selectedTmpl.vmID, int32(newVMID), name,
		int32(memory), int32(cores), ipConfig, start, startAtBoot)
}

func (m VMLaunchModel) fetchTemplatesCmd(node models.Node) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		client, err := managergrpc.NewClient(node.Address, node.APIKey)
		if err != nil {
			return vmLoadedTemplatesMsg{err: err}
		}
		defer client.Close()
		resp, err := client.ListTemplates(ctx)
		if err != nil {
			return vmLoadedTemplatesMsg{err: err}
		}
		var templates []remoteTemplate
		for _, t := range resp.Templates {
			templates = append(templates, remoteTemplate{vmID: t.VmId, name: t.Name, tags: t.Tags})
		}
		return vmLoadedTemplatesMsg{templates: templates}
	}
}

func (m VMLaunchModel) launchVMCmd(templateID, newVMID int32, name string, memory, cores int32, ipConfig string, start, startAtBoot bool) tea.Cmd {
	node := m.selectedNode
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		client, err := managergrpc.NewClient(node.Address, node.APIKey)
		if err != nil {
			return vmLaunchResultMsg{err: err}
		}
		defer client.Close()
		resp, err := client.LaunchVM(ctx, &pb.LaunchVMRequest{
			TemplateId: templateID, NewVmId: newVMID, Name: name,
			Start: start, StartAtBoot: startAtBoot, IpConfig: ipConfig,
			Memory: memory, Cores: cores,
		})
		return vmLaunchResultMsg{resp: resp, err: err}
	}
}

// View renders the VM launch flow.
func (m VMLaunchModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" Launch VM "))
	b.WriteString("\n\n")

	switch m.phase {
	case vmPhaseNodeSelect:
		b.WriteString("  Select a node:\n\n")
		if len(m.nodes) == 0 {
			b.WriteString(styles.MutedStyle.Render("  No nodes configured."))
		}
		for i, n := range m.nodes {
			cursor := "  "
			if i == m.nodeCursor {
				cursor = styles.SelectedStyle.Render("> ")
			}
			label := n.Label()
			if i == m.nodeCursor {
				label = styles.SelectedStyle.Render(label)
			}
			fmt.Fprintf(&b, "  %s%s  %s\n", cursor, label, styles.MutedStyle.Render(n.Address))
		}
		b.WriteString("\n")
		b.WriteString(styles.MutedStyle.Render("  ↑↓: navigate • enter: select"))
		if m.statusMsg != "" {
			b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
		}

	case vmPhaseLoadTemplates:
		b.WriteString(styles.WarningStyle.Render("  Loading templates from ") + m.selectedNode.Label() + "...")

	case vmPhaseTmplSelect:
		fmt.Fprintf(&b, "  Node: %s — Select a template:\n\n", styles.SelectedStyle.Render(m.selectedNode.Label()))
		for i, t := range m.remoteTemplates {
			cursor := "  "
			if i == m.tmplCursor {
				cursor = styles.SelectedStyle.Render("> ")
			}
			label := fmt.Sprintf("VM %d: %s", t.vmID, t.name)
			if i == m.tmplCursor {
				label = styles.SelectedStyle.Render(label)
			}
			tags := ""
			if t.tags != "" {
				tags = styles.MutedStyle.Render("  [" + t.tags + "]")
			}
			fmt.Fprintf(&b, "  %s%s%s\n", cursor, label, tags)
		}
		b.WriteString("\n")
		b.WriteString(styles.MutedStyle.Render("  ↑↓: navigate • enter: select • esc: back"))

	case vmPhaseConfig:
		fmt.Fprintf(&b, "  Clone from: VM %d (%s) on %s\n\n",
			m.selectedTmpl.vmID, m.selectedTmpl.name, m.selectedNode.Label())

		visible := m.visibleFields()
		for vi, fid := range visible {
			f := m.fields[fid]
			cursor := "  "
			if vi == m.cursor {
				cursor = styles.SelectedStyle.Render("> ")
			}

			label := f.label
			if vi == m.cursor {
				label = styles.SelectedStyle.Render(label)
			}

			var value string
			if len(f.options) > 0 {
				// Render select
				var opts []string
				for oi, o := range f.options {
					if oi == f.selIdx {
						opts = append(opts, styles.SuccessStyle.Render("["+o+"]"))
					} else {
						opts = append(opts, styles.MutedStyle.Render(" "+o+" "))
					}
				}
				value = strings.Join(opts, " ")
			} else {
				value = f.input.View()
			}

			fmt.Fprintf(&b, "  %s%-18s %s\n", cursor, label+":", value)
		}

		b.WriteString("\n")
		b.WriteString(styles.MutedStyle.Render("  tab/↑↓: navigate • ←→: toggle select • enter: launch • esc: back"))
		if m.statusMsg != "" {
			b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
		}

	case vmPhaseLaunching:
		b.WriteString(styles.WarningStyle.Render("  Launching VM..."))

	case vmPhaseResult:
		if strings.Contains(m.statusMsg, "created") {
			b.WriteString(styles.SuccessStyle.Render("  " + m.statusMsg))
		} else {
			b.WriteString(styles.ErrorStyle.Render("  " + m.statusMsg))
		}
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  enter/esc: back"))
	}

	return b.String()
}
