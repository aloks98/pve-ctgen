package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

type tmplMode int

const (
	tmplModeList tmplMode = iota
	tmplModeAdd
	tmplModeEdit
	tmplModeShow
)

// TemplatesModel manages the template store view.
type TemplatesModel struct {
	table     components.Table
	db        *store.DB
	names     []string
	mode      tmplMode
	form      components.Form
	editName  string
	detail    string
	statusMsg string
	width     int
	height    int
}

// NewTemplatesModel creates a new TemplatesModel.
func NewTemplatesModel(db *store.DB) TemplatesModel {
	t := components.NewTable(
		[]string{"VM_ID", "NAME", "URL", "TAGS", "CLOUDINIT"},
		[]int{8, 15, 40, 25, 15},
	)
	return TemplatesModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *TemplatesModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
}

// Refresh reloads data from the store.
func (m *TemplatesModel) Refresh() {
	templates, err := m.db.ListTemplates()
	if err != nil {
		return
	}
	rows := make([][]string, len(templates))
	m.names = make([]string, len(templates))
	for i, t := range templates {
		ciName := "-"
		if t.CloudInit != "" {
			ciName = t.CloudInit
		}
		rows[i] = []string{
			fmt.Sprintf("%d", t.VMID),
			t.Name,
			t.URL,
			t.Tags,
			ciName,
		}
		m.names[i] = t.Name
	}
	m.table.SetRows(rows)
	m.mode = tmplModeList
	m.statusMsg = ""
}

// InSubView returns true if the view is in a sub-mode.
func (m *TemplatesModel) InSubView() bool {
	return m.mode != tmplModeList
}

func (m *TemplatesModel) newTemplateForm(isEdit bool) components.Form {
	title := "Add Template"
	if isEdit {
		title = "Edit Template"
	}
	return components.NewForm(title, []components.FormField{
		{Label: "Name", Placeholder: "e.g. ubuntu2404", Required: true},
		{Label: "VM ID", Placeholder: "e.g. 8201 (auto-assigned if empty)"},
		{Label: "URL", Placeholder: "https://...", Required: true},
		{Label: "Checksum URL", Placeholder: "https://... (optional)"},
		{Label: "Tags", Placeholder: "ubuntu,cloudinit"},
		{Label: "Init Config", Placeholder: "ubuntu.yaml or k8s-node.bu (optional)"},
		{Label: "Init Type", Placeholder: "cloudinit or ignition (default: auto from extension)"},
	})
}

// Update handles key events.
func (m TemplatesModel) Update(msg tea.Msg) (TemplatesModel, tea.Cmd) {
	switch m.mode {
	case tmplModeAdd, tmplModeEdit:
		return m.updateFormMode(msg)
	case tmplModeShow:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "backspace" {
				m.mode = tmplModeList
			}
		}
		return m, nil
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "a":
			m.form = m.newTemplateForm(false)
			m.mode = tmplModeAdd
			return m, m.form.Fields[0].BlinkCmd()
		case "e":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				t, err := m.db.GetTemplate(m.names[idx])
				if err == nil {
					m.editName = t.Name
					m.form = m.newTemplateForm(true)
					m.form.SetValue("Name", t.Name)
					m.form.SetValue("VM ID", fmt.Sprintf("%d", t.VMID))
					m.form.SetValue("URL", t.URL)
					m.form.SetValue("Checksum URL", t.ChecksumURL)
					m.form.SetValue("Tags", t.Tags)
					if t.CloudInit != "" {
						m.form.SetValue("Init Config", t.CloudInit)
					}
					if t.InitType != "" {
						m.form.SetValue("Init Type", t.InitType)
					}
					m.mode = tmplModeEdit
					return m, m.form.Fields[0].BlinkCmd()
				}
			}
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				t, err := m.db.GetTemplate(m.names[idx])
				if err == nil {
					var detail strings.Builder
					fmt.Fprintf(&detail, "  Name:          %s\n", t.Name)
					fmt.Fprintf(&detail, "  VM ID:         %d\n", t.VMID)
					fmt.Fprintf(&detail, "  URL:           %s\n", t.URL)
					fmt.Fprintf(&detail, "  Checksum URL:  %s\n", t.ChecksumURL)
					fmt.Fprintf(&detail, "  Tags:          %s\n", t.Tags)
					if t.CloudInit != "" {
						fmt.Fprintf(&detail, "  Init Config:   %s\n", t.CloudInit)
					}
					fmt.Fprintf(&detail, "  Init Type:     %s\n", t.EffectiveInitType())
					m.detail = detail.String()
					m.mode = tmplModeShow
				}
			}
		case "d":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				name := m.names[idx]
				m.db.DeleteTemplate(name)
				m.Refresh()
				m.statusMsg = fmt.Sprintf("Deleted %q", name)
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

func (m TemplatesModel) updateFormMode(msg tea.Msg) (TemplatesModel, tea.Cmd) {
	cmd := m.form.Update(msg)
	if m.form.Cancelled {
		m.mode = tmplModeList
		return m, nil
	}
	if m.form.Submitted {
		name := m.form.Value("Name")
		vmIDStr := m.form.Value("VM ID")
		url := m.form.Value("URL")
		checksumURL := m.form.Value("Checksum URL")
		tags := m.form.Value("Tags")
		ciName := m.form.Value("Init Config")
		initType := strings.TrimSpace(m.form.Value("Init Type"))

		if name == "" || url == "" {
			m.statusMsg = "Name and URL are required"
			m.form.Submitted = false
			return m, nil
		}

		vmID := 0
		if vmIDStr != "" {
			v, err := strconv.Atoi(vmIDStr)
			if err != nil {
				m.statusMsg = "VM ID must be a number"
				m.form.Submitted = false
				return m, nil
			}
			vmID = v
		}

		if ciName != "" {
			if _, err := m.db.GetCloudInit(ciName); err != nil {
				m.statusMsg = fmt.Sprintf("Cloud-init %q not found", ciName)
				m.form.Submitted = false
				return m, nil
			}
		}

		if m.mode == tmplModeAdd {
			t := &models.Template{
				VMID:        vmID,
				Name:        name,
				URL:         url,
				ChecksumURL: checksumURL,
				Tags:        tags,
				CloudInit:   ciName,
				InitType:    initType,
			}
			if _, err := m.db.CreateTemplate(t); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				m.form.Submitted = false
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Added %q", name)
		} else {
			updates := map[string]interface{}{
				"name":         name,
				"vm_id":        vmID,
				"url":          url,
				"checksum_url": checksumURL,
				"tags":         tags,
				"cloudinit":    ciName,
				"init_type":    initType,
			}
			if err := m.db.UpdateTemplate(m.editName, updates); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				m.form.Submitted = false
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Updated %q", name)
		}
		return m, nil
	}
	return m, cmd
}

// View renders the templates store.
func (m TemplatesModel) View() string {
	var b strings.Builder

	switch m.mode {
	case tmplModeAdd, tmplModeEdit:
		b.WriteString("\n")
		b.WriteString(m.form.View())
		if m.statusMsg != "" {
			b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
		}
		return b.String()

	case tmplModeShow:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Template Detail "))
		b.WriteString("\n\n")
		b.WriteString(m.detail)
		b.WriteString("\n")
		b.WriteString(styles.MutedStyle.Render("  esc: back"))
		return b.String()
	}

	// List mode
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" Templates "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.MutedStyle.Render("  enter: details • a: add • e: edit • d: delete"))
	if m.statusMsg != "" {
		b.WriteString("  " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
