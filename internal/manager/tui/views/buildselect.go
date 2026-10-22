package views

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/internal/manager/buildrun"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// StartBuildMsg contains the immutable request snapshots approved in review.
type StartBuildMsg struct {
	Templates []models.Template
	Node      models.Node
	Requests  []*pb.BuildRequest
}

const (
	phaseTemplates = iota
	phaseNodes
	phaseVMIDs
	phaseReview
)

type selTemplate struct {
	tmpl     models.Template
	selected bool
}
type preflightResultMsg struct {
	revision uint64
	checks   []*pb.CheckResult
	err      error
}

type BuildSelectModel struct {
	db                           *store.DB
	templates                    []selTemplate
	nodes                        []models.Node
	selectedNode                 *models.Node
	cursor, phase, width, height int
	statusMsg                    string
	form                         components.Form
	selected                     []models.Template
	requests                     []*pb.BuildRequest
	checks                       []*pb.CheckResult
	checking, ready, submitted   bool
	revision                     uint64
	cancel                       context.CancelFunc
	review                       viewport.Model
	// The command seam keeps remote checks outside the UI update loop.
	preflight func(models.Node, []*pb.BuildRequest, uint64, context.Context) tea.Cmd
}

func NewBuildSelectModel(db *store.DB) BuildSelectModel {
	return BuildSelectModel{db: db, preflight: checkBuilds, review: viewport.New(80, 16)}
}
func (m *BuildSelectModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.form.SetSize(w, max(6, h-6))
	m.review.Width = max(1, w)
	m.review.Height = max(1, h-6)
	m.renderReview()
}
func (m *BuildSelectModel) Refresh() {
	if m.cancel != nil {
		m.cancel()
	}
	m.revision++
	m.phase = phaseTemplates
	m.cursor = 0
	m.statusMsg = ""
	m.selectedNode = nil
	m.selected = nil
	m.requests = nil
	m.checks = nil
	m.checking = false
	m.ready = false
	m.submitted = false
	templates, err := m.db.ListTemplates()
	if err != nil {
		m.statusMsg = err.Error()
	}
	m.templates = nil
	for _, t := range templates {
		m.templates = append(m.templates, selTemplate{tmpl: t})
	}
	m.nodes, err = m.db.ListNodes()
	if err != nil {
		m.statusMsg = err.Error()
	}
}
func (m *BuildSelectModel) SelectTemplate(name string) {
	m.Refresh()
	for i := range m.templates {
		if m.templates[i].tmpl.Name == name {
			m.templates[i].selected = true
			m.phase = phaseNodes
			return
		}
	}
}
func (m *BuildSelectModel) InSubView() bool { return m.phase > phaseTemplates }
func (m BuildSelectModel) Update(msg tea.Msg) (BuildSelectModel, tea.Cmd) {
	if result, ok := msg.(preflightResultMsg); ok {
		if result.revision != m.revision || m.phase != phaseReview {
			return m, nil
		}
		m.checking = false
		if m.cancel != nil {
			m.cancel()
		}
		m.checks = result.checks
		m.ready = result.err == nil && len(result.checks) > 0
		for _, c := range result.checks {
			if !c.Ok {
				m.ready = false
			}
		}
		m.statusMsg = ""
		if result.err != nil {
			m.statusMsg = result.err.Error()
		}
		m.renderReview()
		return m, nil
	}
	if m.phase == phaseReview {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "esc":
				if m.cancel != nil {
					m.cancel()
				}
				m.revision++
				m.phase = phaseVMIDs
				m.form.Submitted = false
				m.ready = false
				m.checking = false
				m.statusMsg = ""
				return m, nil
			case "enter":
				if !m.ready || m.submitted {
					return m, nil
				}
				m.submitted = true
				request := StartBuildMsg{Templates: m.selected, Node: *m.selectedNode, Requests: m.requests}
				return m, func() tea.Msg { return request }
			case "r":
				if !m.checking {
					cmd := m.beginPreflight()
					return m, cmd
				}
			}
		}
		var cmd tea.Cmd
		m.review, cmd = m.review.Update(msg)
		return m, cmd
	}
	if m.phase == phaseVMIDs {
		cmd := m.form.Update(msg)
		if m.form.Cancelled {
			m.phase = phaseNodes
			m.cursor = 0
			m.statusMsg = ""
			return m, nil
		}
		if m.form.Submitted {
			m.form.Submitted = false
			return m.prepareReview()
		}
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		limit := len(m.templates)
		if m.phase == phaseNodes {
			limit = len(m.nodes)
		}
		switch k.String() {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, limit-1), m.cursor+1)
		case " ":
			if m.phase == phaseTemplates && m.cursor < len(m.templates) {
				m.templates[m.cursor].selected = !m.templates[m.cursor].selected
			}
		case "a":
			if m.phase == phaseTemplates {
				all := true
				for _, t := range m.templates {
					all = all && t.selected
				}
				for i := range m.templates {
					m.templates[i].selected = !all
				}
			}
		case "esc":
			if m.phase == phaseNodes {
				m.phase = phaseTemplates
				m.cursor = 0
				m.statusMsg = ""
			}
		case "enter":
			if m.phase == phaseTemplates {
				m.selected = nil
				for _, t := range m.templates {
					if t.selected {
						m.selected = append(m.selected, t.tmpl)
					}
				}
				if len(m.selected) == 0 {
					m.statusMsg = "Select at least one template with space."
					return m, nil
				}
				m.phase = phaseNodes
				m.cursor = 0
				m.statusMsg = ""
			} else {
				if len(m.nodes) == 0 {
					m.statusMsg = "No nodes configured. Add a node from the workspace."
					return m, nil
				}
				node := m.nodes[m.cursor]
				m.selectedNode = &node
				m.selected = nil
				var fields []components.FormField
				for _, t := range m.templates {
					if t.selected {
						m.selected = append(m.selected, t.tmpl)
						fields = append(fields, components.FormField{Label: t.tmpl.Name, Value: strconv.Itoa(t.tmpl.VMID), Required: true})
					}
				}
				if len(fields) == 0 {
					m.phase = phaseTemplates
					m.statusMsg = "Select a template first."
					return m, nil
				}
				m.form = components.NewForm("Target VM IDs", fields)
				m.form.SetSize(m.width, max(6, m.height-6))
				m.phase = phaseVMIDs
				m.statusMsg = ""
				return m, m.form.Fields[0].BlinkCmd()
			}
		}
	}
	return m, nil
}
func (m BuildSelectModel) prepareReview() (BuildSelectModel, tea.Cmd) {
	m.requests = nil
	seen := map[int]bool{}
	for i := range m.selected {
		id, err := strconv.Atoi(m.form.Value(m.selected[i].Name))
		if err != nil || id < 100 || id > 999999999 {
			m.statusMsg = "VM IDs must be between 100 and 999999999."
			return m, nil
		}
		if seen[id] {
			m.statusMsg = fmt.Sprintf("Duplicate target VM ID %d.", id)
			return m, nil
		}
		seen[id] = true
		tmpl := m.selected[i]
		tmpl.VMID = id
		req, err := buildrun.Prepare(m.db, tmpl, nil)
		if err != nil {
			m.statusMsg = tmpl.Name + ": " + err.Error()
			return m, nil
		}
		m.selected[i] = tmpl
		m.requests = append(m.requests, req)
	}
	m.phase = phaseReview
	m.submitted = false
	m.review.GotoTop()
	cmd := m.beginPreflight()
	return m, cmd
}
func (m *BuildSelectModel) beginPreflight() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.revision++
	m.ready = false
	m.checking = true
	m.checks = nil
	m.statusMsg = ""
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	m.cancel = cancel
	m.renderReview()
	return m.preflight(*m.selectedNode, m.requests, m.revision, ctx)
}
func checkBuilds(node models.Node, requests []*pb.BuildRequest, revision uint64, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		result := preflightResultMsg{revision: revision}
		client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
		if err != nil {
			result.err = err
			return result
		}
		defer client.Close()
		for _, req := range requests {
			response, err := client.Preflight(ctx, &pb.PreflightRequest{Profile: req.Options["profile"], Build: req})
			if err != nil {
				result.err = err
				return result
			}
			if len(response.Checks) == 0 {
				result.err = fmt.Errorf("no preflight results for %s", req.Image.Name)
				return result
			}
			for _, check := range response.Checks {
				result.checks = append(result.checks, &pb.CheckResult{Name: req.Image.Name + " / " + check.Name, Ok: check.Ok, Message: check.Message})
			}
		}
		return result
	}
}
func (m *BuildSelectModel) renderReview() {
	if m.selectedNode == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Target node  %s\n\n", m.selectedNode.Label())
	for _, req := range m.requests {
		profile := req.Options["profile"]
		if profile == "" {
			profile = "default"
		}
		fmt.Fprintf(&b, "%s -> VMID %d\n  Profile: %s | Init: %s\n", req.Image.Name, req.Image.Id, profile, req.InitType)
	}
	b.WriteString("\nExisting VM policy: stop if VMID exists.\n\nPREFLIGHT\n")
	if m.checking {
		b.WriteString("[..] Checking node, storage and target IDs...\n")
	}
	for _, check := range m.checks {
		tag := styles.StatusTag("completed")
		if !check.Ok {
			tag = styles.StatusTag("failed")
		}
		fmt.Fprintf(&b, "%s %s\n     %s\n", tag, check.Name, check.Message)
	}
	if m.statusMsg != "" {
		b.WriteString(styles.ErrorStyle.Render(m.statusMsg))
	}
	m.review.SetContent(ansi.Hardwrap(b.String(), max(1, m.width), true))
}
func (m BuildSelectModel) View() string {
	stages := []string{"Select", "Node", "VM IDs", "Review"}
	for i := range stages {
		if i == m.phase {
			stages[i] = styles.SelectedStyle.Render("[" + stages[i] + "]")
		}
	}
	heading := styles.BoldStyle.Render("Build a template") + "\n" + strings.Join(stages, " > ") + "\n\n"
	if m.phase == phaseReview {
		hint := "Checking... | esc back"
		if !m.checking {
			hint = "Blocked | r retry | esc back"
		}
		if m.ready {
			hint = "enter start build | esc edit"
		}
		return heading + m.review.View() + "\n" + styles.DimStyle.Render(hint+" | j/k scroll")
	}
	if m.phase == phaseVMIDs {
		return heading + m.form.View() + "\n" + styles.ErrorStyle.Render(m.statusMsg)
	}
	var rows []string
	if m.phase == phaseTemplates {
		for _, t := range m.templates {
			check := "[ ]"
			if t.selected {
				check = "[x]"
			}
			rows = append(rows, fmt.Sprintf("%s %s / %d", check, t.tmpl.Name, t.tmpl.VMID))
		}
	} else {
		for _, n := range m.nodes {
			rows = append(rows, n.Label()+" / "+n.Address)
		}
	}
	if len(rows) == 0 {
		rows = append(rows, "No items. Add definitions and nodes from the workspace.")
	}
	visible := max(1, m.height-8)
	start := max(0, m.cursor-visible+1)
	for i := start; i < min(len(rows), start+visible); i++ {
		line := "  " + rows[i]
		if i == m.cursor {
			line = styles.SelectedStyle.Render("> " + rows[i])
		}
		heading += line + "\n"
	}
	hint := "space select | a all | enter choose node"
	if m.phase == phaseNodes {
		hint = "enter configure VM IDs | esc back"
	}
	return heading + "\n" + styles.DimStyle.Render(hint) + "\n" + styles.ErrorStyle.Render(m.statusMsg)
}
