package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	tmplModeDelete
)

// BuildTemplateMsg opens the guided build flow with a selected definition.
type BuildTemplateMsg struct{ Name string }

// TemplatesModel manages the template store view.
type TemplatesModel struct {
	table      components.Table
	db         *store.DB
	names      []string
	mode       tmplMode
	form       components.Form
	editName   string
	detail     string
	statusMsg  string
	width      int
	height     int
	templates  []models.Template
	search     textinput.Model
	searching  bool
	detailView viewport.Model
}

// NewTemplatesModel creates a new TemplatesModel.
func NewTemplatesModel(db *store.DB) TemplatesModel {
	t := components.NewTable(
		[]string{"NAME", "VMID", "INIT", "PROFILE"},
		[]int{28, 8, 12, 15},
	)
	t.EmptyMessage = "  No templates yet. Press a to add, or run: pvectgen manager import"
	search := textinput.New()
	search.Placeholder = "Filter templates"
	search.CharLimit = 100
	return TemplatesModel{table: t, db: db, search: search}
}

// SetSize sets the view dimensions.
func (m *TemplatesModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = max(4, h-7)
	m.width = w
	m.height = h
	m.search.Width = max(10, w-8)
	if h >= 20 {
		m.table.Height = max(4, h-15)
	}
	m.detailView.Width = max(1, w)
	m.detailView.Height = max(1, h-5)
	m.detailView.SetContent(ansi.Hardwrap(m.detail, max(1, w), true))
}

// Refresh reloads data from the store.
func (m *TemplatesModel) Refresh() {
	templates, err := m.db.ListTemplates()
	if err != nil {
		m.statusMsg = err.Error()
		return
	}
	m.templates = templates
	m.filter()
	m.mode = tmplModeList
	m.statusMsg = ""
}

func (m *TemplatesModel) filter() {
	var rows [][]string
	m.names = nil
	query := strings.ToLower(m.search.Value())
	for _, t := range m.templates {
		if !strings.Contains(strings.ToLower(t.Name+" "+t.Tags+" "+t.Profile+" "+t.EffectiveInitType()), query) {
			continue
		}
		profile := t.Profile
		if profile == "" {
			profile = "default"
		}
		rows = append(rows, []string{t.Name, fmt.Sprint(t.VMID), t.EffectiveInitType(), profile})
		m.names = append(m.names, t.Name)
	}
	m.table.SetRows(rows)
}

// InSubView returns true if the view is in a sub-mode.
func (m *TemplatesModel) InSubView() bool {
	return m.mode != tmplModeList || m.searching
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
		{Label: "Checksum URL", Placeholder: "https://...", Required: true},
		{Label: "Tags", Placeholder: "ubuntu,cloudinit"},
		{Label: "Init Config", Placeholder: "ubuntu.yaml or k8s-node.bu (optional)"},
		{Label: "Init Type", Placeholder: "cloudinit or ignition (default: auto from extension)"},
		{Label: "Profile", Placeholder: "default (node build profile)"},
	})
}

// Update handles key events.
func (m TemplatesModel) Update(msg tea.Msg) (TemplatesModel, tea.Cmd) {
	if m.searching {
		if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "enter" || k.String() == "esc") {
			m.searching = false
			m.search.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.table.Cursor = 0
		m.filter()
		return m, cmd
	}
	if m.mode == tmplModeDelete {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "y":
				name := m.editName
				if err := m.db.DeleteTemplate(name); err != nil {
					m.statusMsg = err.Error()
				} else {
					m.Refresh()
					m.statusMsg = "Deleted " + name
				}
				m.mode = tmplModeList
			case "n", "esc":
				m.mode = tmplModeList
			}
		}
		return m, nil
	}
	switch m.mode {
	case tmplModeAdd, tmplModeEdit:
		return m.updateFormMode(msg)
	case tmplModeShow:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "backspace" {
				m.mode = tmplModeList
			}
		}
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		return m, cmd
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "/":
			m.searching = true
			return m, m.search.Focus()
		case "b":
			if idx := m.table.SelectedRow(); idx >= 0 && idx < len(m.names) {
				name := m.names[idx]
				return m, func() tea.Msg { return BuildTemplateMsg{Name: name} }
			}
		case "r":
			m.Refresh()
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
					m.form.SetValue("Profile", t.Profile)
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
					fmt.Fprintf(&detail, "  Profile:       %s\n", t.Profile)
					m.detail = detail.String()
					m.detailView.SetContent(ansi.Hardwrap(m.detail, max(1, m.width), true))
					m.detailView.GotoTop()
					m.mode = tmplModeShow
				}
			}
		case "d":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				name := m.names[idx]
				m.editName = name
				m.mode = tmplModeDelete
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
				Profile:     strings.TrimSpace(m.form.Value("Profile")),
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
				"profile":      strings.TrimSpace(m.form.Value("Profile")),
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
	case tmplModeDelete:
		return styles.WarningStyle.Render("Delete template definition "+m.editName+"?") + "\n\nThis removes the local definition only.\nExisting Proxmox VMs are unaffected.\n\ny confirm / n or esc cancel"
	case tmplModeAdd, tmplModeEdit:
		b.WriteString("\n")
		m.form.SetSize(m.width, m.height-3)
		b.WriteString(m.form.View())
		if m.statusMsg != "" {
			b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
		}
		return b.String()

	case tmplModeShow:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Template Detail "))
		b.WriteString("\n\n")
		b.WriteString(m.detailView.View())
		b.WriteString("\n")
		b.WriteString(styles.MutedStyle.Render("  esc: back"))
		return b.String()
	}

	// List mode
	subtitle := fmt.Sprintf("%d definitions", len(m.names))
	if m.search.Value() != "" {
		subtitle += " · filter: " + m.search.Value()
	}
	b.WriteString(styles.PageHeader("Templates", subtitle, "b  Build…", m.width))
	if m.searching {
		// Replace the subtitle in place so filtering does not change the layout.
		b.Reset()
		b.WriteString(styles.PageHeader("Templates", "/ "+m.search.View(), "b  Build…", m.width))
	}
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	if idx := m.table.SelectedRow(); idx >= 0 && idx < len(m.names) && m.height >= 20 {
		for _, t := range m.templates {
			if t.Name != m.names[idx] {
				continue
			}
			profile := t.Profile
			if profile == "" {
				profile = "Node default"
			}
			init := t.CloudInit
			if init == "" {
				init = "None"
			}
			tags := t.Tags
			if tags == "" {
				tags = "None"
			}
			field := func(label, value string, width int) string {
				return styles.Fit(styles.DimStyle.Render(label), width, 1) + "\n" + styles.Fit(value, width, 1)
			}
			col := max(1, (m.width-3)/2)
			left := field("Template", fmt.Sprintf("%s · VMID %d", t.Name, t.VMID), col) + "\n\n" + field("Profile", profile, col)
			right := field("Initialization", init+" · "+t.EffectiveInitType(), m.width-col-3) + "\n\n" + field("Tags", tags, m.width-col-3)
			b.WriteString(styles.Rule(m.width) + "\n\n")
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right))
			b.WriteString("\n\n")
			break
		}
	}
	b.WriteString(styles.DimStyle.Render("/ filter  a add  e edit  d delete  enter details"))
	if m.statusMsg != "" {
		b.WriteString("  " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
