package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/aloks98/pve-ctgen/internal/shared/token"
)

type nodesMode int

const (
	nodesModeList nodesMode = iota
	nodesModeAdd
	nodesModeHealth
)

// nodeHealthMsg is sent when a health check completes.
type nodeHealthMsg struct {
	label   string
	healthy bool
	version string
	storage []string
	err     error
}

// NodesModel manages the nodes TUI view.
type NodesModel struct {
	table     components.Table
	db        *store.DB
	nodes     []models.Node
	mode      nodesMode
	form      components.Form
	statusMsg string
	healthMsg string
	width     int
	height    int
}

// NewNodesModel creates a new NodesModel.
func NewNodesModel(db *store.DB) NodesModel {
	t := components.NewTable(
		[]string{"NAME", "DISPLAY NAME", "ADDRESS", "ADDED"},
		[]int{18, 18, 22, 12},
	)
	return NodesModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *NodesModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
}

// Refresh reloads data from the store.
func (m *NodesModel) Refresh() {
	nodes, err := m.db.ListNodes()
	if err != nil {
		return
	}
	m.nodes = nodes
	rows := make([][]string, len(nodes))
	for i, n := range nodes {
		rows[i] = []string{n.Name, n.DisplayName, n.Address, n.CreatedAt.Format("2006-01-02")}
	}
	m.table.SetRows(rows)
	m.mode = nodesModeList
	m.statusMsg = ""
}

// InSubView returns true if the view is in a sub-mode.
func (m *NodesModel) InSubView() bool {
	return m.mode != nodesModeList
}

// Update handles key events and async messages.
func (m NodesModel) Update(msg tea.Msg) (NodesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case addNodeResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Failed: %v", msg.err)
			m.mode = nodesModeList
			return m, nil
		}
		if _, err := m.db.CreateNode(msg.name, msg.displayName, msg.address, msg.apiKey); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			m.mode = nodesModeList
			return m, nil
		}
		label := msg.name
		if msg.displayName != "" {
			label = fmt.Sprintf("%s (%s)", msg.displayName, msg.name)
		}
		m.Refresh()
		m.statusMsg = fmt.Sprintf("Added %s — %s", label, msg.version)
		return m, nil

	case nodeHealthMsg:
		if msg.err != nil {
			m.healthMsg = fmt.Sprintf("%s: %s", msg.label, styles.ErrorStyle.Render(fmt.Sprintf("ERROR: %v", msg.err)))
		} else {
			status := styles.SuccessStyle.Render("HEALTHY")
			if !msg.healthy {
				status = styles.ErrorStyle.Render("UNHEALTHY")
			}
			m.healthMsg = fmt.Sprintf("%s: %s  %s  storage: %v", msg.label, status, msg.version, msg.storage)
		}
		m.mode = nodesModeHealth
		return m, nil
	}

	switch m.mode {
	case nodesModeAdd:
		return m.updateAddMode(msg)
	case nodesModeHealth:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "enter" || km.String() == "backspace" {
				m.mode = nodesModeList
			}
		}
		return m, nil
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "a":
			m.form = components.NewForm("Add Node", []components.FormField{
				{Label: "Token", Placeholder: "pvectgen-token://...", Required: true},
				{Label: "Display Name", Placeholder: "e.g. prod-node-1 (optional)"},
			})
			m.mode = nodesModeAdd
			return m, m.form.Fields[0].BlinkCmd()
		case "d":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.nodes) {
				n := m.nodes[idx]
				m.db.DeleteNode(n.Name)
				m.Refresh()
				m.statusMsg = fmt.Sprintf("Removed %s", n.Label())
			}
		case "h":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.nodes) {
				n := m.nodes[idx]
				m.healthMsg = fmt.Sprintf("Checking %s...", n.Label())
				m.mode = nodesModeHealth
				return m, m.checkHealthCmd(n)
			}
		case "e":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.nodes) {
				n := m.nodes[idx]
				m.form = components.NewForm("Edit Node Display Name", []components.FormField{
					{Label: "Display Name", Placeholder: "e.g. prod-node-1", Value: n.DisplayName},
				})
				m.mode = nodesModeAdd // reuse add mode for the form
				// tag so we know it's an edit
				m.statusMsg = "edit:" + n.Name
				return m, m.form.Fields[0].BlinkCmd()
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

func (m NodesModel) updateAddMode(msg tea.Msg) (NodesModel, tea.Cmd) {
	cmd := m.form.Update(msg)
	if m.form.Cancelled {
		m.mode = nodesModeList
		m.statusMsg = ""
		return m, nil
	}
	if m.form.Submitted {
		// Check if this is an edit (display name only)
		if strings.HasPrefix(m.statusMsg, "edit:") {
			nodeName := strings.TrimPrefix(m.statusMsg, "edit:")
			displayName := m.form.Value("Display Name")
			if err := m.db.UpdateNodeDisplayName(nodeName, displayName); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				m.form.Submitted = false
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Updated display name for %s", nodeName)
			return m, nil
		}

		// Add new node from token
		tok := m.form.Value("Token")
		displayName := m.form.Value("Display Name")

		if tok == "" {
			m.statusMsg = "Token is required"
			m.form.Submitted = false
			return m, nil
		}

		info, err := token.Decode(tok)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Invalid token: %v", err)
			m.form.Submitted = false
			return m, nil
		}

		// Verify connectivity
		m.statusMsg = "Verifying connection..."
		return m, m.addNodeCmd(info, displayName)
	}
	return m, cmd
}

// addNodeResultMsg is sent when node add verification completes.
type addNodeResultMsg struct {
	name        string
	displayName string
	address     string
	apiKey      string
	version     string
	err         error
}

func (m NodesModel) addNodeCmd(info token.ConnectionInfo, displayName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := managergrpc.NewClient(info.Address, info.APIKey)
		if err != nil {
			return addNodeResultMsg{err: fmt.Errorf("connect: %w", err)}
		}
		defer client.Close()

		resp, err := client.Health(ctx)
		if err != nil {
			return addNodeResultMsg{err: fmt.Errorf("health check: %w", err)}
		}

		return addNodeResultMsg{
			name:        info.Name,
			displayName: displayName,
			address:     info.Address,
			apiKey:      info.APIKey,
			version:     resp.ProxmoxVersion,
		}
	}
}

func (m NodesModel) checkHealthCmd(node models.Node) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := managergrpc.NewClient(node.Address, node.APIKey)
		if err != nil {
			return nodeHealthMsg{label: node.Label(), err: err}
		}
		defer client.Close()

		resp, err := client.Health(ctx)
		if err != nil {
			return nodeHealthMsg{label: node.Label(), err: err}
		}

		return nodeHealthMsg{
			label:   node.Label(),
			healthy: resp.Healthy,
			version: resp.ProxmoxVersion,
			storage: resp.AvailableStorage,
		}
	}
}

// View renders the nodes view.
func (m NodesModel) View() string {
	var b strings.Builder

	switch m.mode {
	case nodesModeAdd:
		b.WriteString("\n")
		b.WriteString(m.form.View())
		if m.statusMsg != "" && !strings.HasPrefix(m.statusMsg, "edit:") {
			b.WriteString("\n  " + styles.MutedStyle.Render(m.statusMsg))
		}
		return b.String()

	case nodesModeHealth:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Node Health "))
		b.WriteString("\n\n")
		b.WriteString("  " + m.healthMsg)
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  esc: back"))
		return b.String()
	}

	// List mode
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" Nodes "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.MutedStyle.Render("  a: add • e: edit name • d: delete • h: health check"))
	if m.statusMsg != "" {
		b.WriteString("  " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
