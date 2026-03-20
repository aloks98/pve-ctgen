package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
)

type ciMode int

const (
	ciModeList ciMode = iota
	ciModeView
	ciModeAdd
	ciModeEdit
)

// CloudInitModel manages the cloud-init store view.
type CloudInitModel struct {
	table    components.Table
	db       *store.DB
	names    []string
	mode     ciMode
	detail   string
	editName string

	// Add/Edit form
	nameInput    components.Form
	contentArea  textarea.Model
	formPhase    int // 0=name form, 1=content editing
	statusMsg    string
	width        int
	height       int
}

// NewCloudInitModel creates a new CloudInitModel.
func NewCloudInitModel(db *store.DB) CloudInitModel {
	t := components.NewTable(
		[]string{"NAME", "CREATED", "UPDATED"},
		[]int{25, 15, 15},
	)
	ta := textarea.New()
	ta.Placeholder = "Paste or type your cloud-init YAML here..."
	ta.CharLimit = 0
	ta.ShowLineNumbers = true

	return CloudInitModel{table: t, db: db, contentArea: ta}
}

// SetSize sets the view dimensions.
func (m *CloudInitModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
	m.contentArea.SetWidth(w - 6)
	m.contentArea.SetHeight(h - 10)
}

// Refresh reloads data from the store.
func (m *CloudInitModel) Refresh() {
	configs, err := m.db.ListCloudInits()
	if err != nil {
		return
	}
	rows := make([][]string, len(configs))
	m.names = make([]string, len(configs))
	for i, c := range configs {
		rows[i] = []string{c.Name, c.CreatedAt.Format("2006-01-02"), c.UpdatedAt.Format("2006-01-02")}
		m.names[i] = c.Name
	}
	m.table.SetRows(rows)
	m.mode = ciModeList
	m.statusMsg = ""
}

// InSubView returns true if the view is in a sub-mode that should capture esc/q.
func (m *CloudInitModel) InSubView() bool {
	return m.mode != ciModeList
}

// Update handles key events.
func (m CloudInitModel) Update(msg tea.Msg) (CloudInitModel, tea.Cmd) {
	switch m.mode {
	case ciModeView:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "backspace" {
				m.mode = ciModeList
			}
		}
		return m, nil

	case ciModeAdd:
		return m.updateAddMode(msg)

	case ciModeEdit:
		return m.updateEditMode(msg)
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				c, err := m.db.GetCloudInit(m.names[idx])
				if err == nil {
					m.detail = c.Content
					m.mode = ciModeView
				}
			}
		case "a":
			m.nameInput = components.NewForm("Add Cloud-Init Config", []components.FormField{
				{Label: "Name", Placeholder: "e.g. ubuntu.yaml", Required: true},
			})
			m.contentArea.Reset()
			m.contentArea.SetValue("")
			m.formPhase = 0
			m.mode = ciModeAdd
			return m, m.nameInput.Fields[0].BlinkCmd()
		case "e":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				c, err := m.db.GetCloudInit(m.names[idx])
				if err == nil {
					m.editName = c.Name
					m.contentArea.Reset()
					m.contentArea.SetValue(c.Content)
					m.contentArea.Focus()
					m.mode = ciModeEdit
					return m, textarea.Blink
				}
			}
		case "d":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				m.db.DeleteCloudInit(m.names[idx])
				m.Refresh()
				m.statusMsg = fmt.Sprintf("Deleted %q", m.names[idx])
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

func (m CloudInitModel) updateAddMode(msg tea.Msg) (CloudInitModel, tea.Cmd) {
	if m.formPhase == 0 {
		// Name input phase
		cmd := m.nameInput.Update(msg)
		if m.nameInput.Cancelled {
			m.mode = ciModeList
			return m, nil
		}
		if m.nameInput.Submitted {
			name := m.nameInput.Value("Name")
			if name == "" {
				m.statusMsg = "Name is required"
				m.nameInput.Submitted = false
				return m, nil
			}
			m.formPhase = 1
			m.contentArea.Focus()
			return m, textarea.Blink
		}
		return m, cmd
	}

	// Content editing phase
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+s":
			name := m.nameInput.Value("Name")
			content := m.contentArea.Value()
			if content == "" {
				m.statusMsg = "Content cannot be empty"
				return m, nil
			}
			warnings, verr := cloudinit.ValidateStrict(content)
			if verr != nil {
				m.statusMsg = fmt.Sprintf("Invalid YAML: %v", verr)
				return m, nil
			}
			if len(warnings) > 0 {
				m.statusMsg = fmt.Sprintf("Warning: %s (saving anyway)", warnings[0])
			}
			if _, err := m.db.CreateCloudInit(name, content); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Added %q", name)
			return m, nil
		case "esc":
			m.mode = ciModeList
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.contentArea, cmd = m.contentArea.Update(msg)
	return m, cmd
}

func (m CloudInitModel) updateEditMode(msg tea.Msg) (CloudInitModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+s":
			content := m.contentArea.Value()
			if content == "" {
				m.statusMsg = "Content cannot be empty"
				return m, nil
			}
			warnings, verr := cloudinit.ValidateStrict(content)
			if verr != nil {
				m.statusMsg = fmt.Sprintf("Invalid YAML: %v", verr)
				return m, nil
			}
			if len(warnings) > 0 {
				m.statusMsg = fmt.Sprintf("Warning: %s (saving anyway)", warnings[0])
			}
			if err := m.db.UpdateCloudInit(m.editName, content); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Updated %q", m.editName)
			return m, nil
		case "esc":
			m.mode = ciModeList
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.contentArea, cmd = m.contentArea.Update(msg)
	return m, cmd
}

// View renders the cloud-init store.
func (m CloudInitModel) View() string {
	var b strings.Builder

	switch m.mode {
	case ciModeView:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Cloud-Init Config Detail "))
		b.WriteString("\n\n")
		b.WriteString(m.detail)
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  esc: back"))
		return b.String()

	case ciModeAdd:
		if m.formPhase == 0 {
			return "\n" + m.nameInput.View()
		}
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" Add: %s ", m.nameInput.Value("Name"))))
		b.WriteString("\n\n")
		b.WriteString("  " + m.contentArea.View())
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  ctrl+s: save • esc: cancel"))
		if m.statusMsg != "" {
			b.WriteString("  " + styles.ErrorStyle.Render(m.statusMsg))
		}
		return b.String()

	case ciModeEdit:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" Edit: %s ", m.editName)))
		b.WriteString("\n\n")
		b.WriteString("  " + m.contentArea.View())
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  ctrl+s: save • esc: cancel"))
		if m.statusMsg != "" {
			b.WriteString("  " + styles.ErrorStyle.Render(m.statusMsg))
		}
		return b.String()
	}

	// List mode
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" Cloud-Init Configs "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.MutedStyle.Render("  enter: view • a: add • e: edit • d: delete"))
	if m.statusMsg != "" {
		b.WriteString("  " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
