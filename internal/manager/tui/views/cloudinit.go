package views

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
)

type ciMode int

const (
	ciModeList ciMode = iota
	ciModeView
	ciModeAdd // name form phase
)

// editorFinishedMsg is sent when the external editor exits.
type editorFinishedMsg struct {
	tmpPath string
	name    string
	isNew   bool
	err     error
}

// CloudInitModel manages the cloud-init store view.
type CloudInitModel struct {
	table     components.Table
	db        *store.DB
	names     []string
	mode      ciMode
	detail    string
	editName  string
	nameInput components.Form
	statusMsg string
	width     int
	height    int
}

// NewCloudInitModel creates a new CloudInitModel.
func NewCloudInitModel(db *store.DB) CloudInitModel {
	t := components.NewTable(
		[]string{"NAME", "CREATED", "UPDATED"},
		[]int{25, 15, 15},
	)
	return CloudInitModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *CloudInitModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
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
	switch msg := msg.(type) {
	case editorFinishedMsg:
		return m.handleEditorFinished(msg)
	case editorDoneMsg:
		return m.handleEditorDone(msg)
	}

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
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				c, err := m.db.GetCloudInit(m.names[idx])
				if err == nil {
					m.editName = c.Name
					m.detail = c.Content
					m.mode = ciModeView
				}
			}
		case "a":
			m.nameInput = components.NewForm("Add Cloud-Init Config", []components.FormField{
				{Label: "Name", Placeholder: "e.g. ubuntu.yaml", Required: true},
			})
			m.mode = ciModeAdd
			return m, m.nameInput.Fields[0].BlinkCmd()
		case "e":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				name := m.names[idx]
				c, err := m.db.GetCloudInit(name)
				if err == nil {
					return m, m.openEditor(name, c.Content, false)
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
		// Check if name already exists
		if _, err := m.db.GetCloudInit(name); err == nil {
			m.statusMsg = fmt.Sprintf("%q already exists", name)
			m.nameInput.Submitted = false
			return m, nil
		}
		// Open editor with a cloud-config template
		template := "#cloud-config\n# " + name + "\n\n"
		m.mode = ciModeList // reset mode before exec
		return m, m.openEditor(name, template, true)
	}
	return m, cmd
}

// openEditor writes content to a temp file and launches $EDITOR (default: nvim).
func (m CloudInitModel) openEditor(name, content string, isNew bool) tea.Cmd {
	return func() tea.Msg {
		tmpFile, err := os.CreateTemp("", "pvectgen-*.yaml")
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("create temp file: %w", err), name: name, isNew: isNew}
		}

		if _, err := tmpFile.WriteString(content); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return editorFinishedMsg{err: fmt.Errorf("write temp file: %w", err), name: name, isNew: isNew}
		}
		tmpFile.Close()

		return editorFinishedMsg{tmpPath: tmpFile.Name(), name: name, isNew: isNew}
	}
}

// editorReadyMsg carries the temp path so we can launch tea.Exec.
func (m CloudInitModel) handleEditorFinished(msg editorFinishedMsg) (CloudInitModel, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = msg.err.Error()
		return m, nil
	}

	if msg.tmpPath != "" {
		// We have a temp file ready — launch the editor via tea.Exec
		m.editName = msg.name
		editor := fileutil.Editor()
		c := exec.Command(editor, msg.tmpPath)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return editorDoneMsg{tmpPath: msg.tmpPath, name: msg.name, isNew: msg.isNew, err: err}
		})
	}

	return m, nil
}

// editorDoneMsg is sent after the editor process exits.
type editorDoneMsg struct {
	tmpPath string
	name    string
	isNew   bool
	err     error
}

// handleEditorDone processes the result after the editor exits.
func (m CloudInitModel) handleEditorDone(msg editorDoneMsg) (CloudInitModel, tea.Cmd) {
	defer os.Remove(msg.tmpPath)

	if msg.err != nil {
		m.statusMsg = fmt.Sprintf("Editor exited with error: %v", msg.err)
		return m, nil
	}

	content, err := os.ReadFile(msg.tmpPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Failed to read file: %v", err)
		return m, nil
	}

	contentStr := string(content)
	if strings.TrimSpace(contentStr) == "" {
		m.statusMsg = "Empty content — not saved"
		return m, nil
	}

	// Validate YAML
	warnings, verr := cloudinit.ValidateStrict(contentStr)
	if verr != nil {
		m.statusMsg = fmt.Sprintf("Invalid YAML: %v — not saved", verr)
		return m, nil
	}

	if msg.isNew {
		if _, err := m.db.CreateCloudInit(msg.name, contentStr); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Added %q", msg.name)
	} else {
		if err := m.db.UpdateCloudInit(msg.name, contentStr); err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Updated %q", msg.name)
	}

	if len(warnings) > 0 {
		m.statusMsg += fmt.Sprintf(" (warning: %s)", warnings[0])
	}

	m.Refresh()
	return m, nil
}

// View renders the cloud-init store.
func (m CloudInitModel) View() string {
	var b strings.Builder

	switch m.mode {
	case ciModeView:
		b.WriteString("\n")
		b.WriteString(styles.SectionStyle.Render(" Cloud-Init: " + m.editName + " "))
		b.WriteString("\n\n")
		b.WriteString(m.detail)
		b.WriteString("\n\n")
		b.WriteString(styles.HelpBar(
			styles.HelpEntry("esc", "back"),
		))
		return b.String()

	case ciModeAdd:
		return "\n" + m.nameInput.View()
	}

	// List mode
	b.WriteString("\n")
	b.WriteString(styles.SectionStyle.Render(" Cloud-Init Configs "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.HelpBar(
		styles.HelpEntry("enter", "view"),
		styles.HelpEntry("a", "add"),
		styles.HelpEntry("e", "edit"),
		styles.HelpEntry("d", "delete"),
	))
	if m.statusMsg != "" {
		b.WriteString("\n " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
