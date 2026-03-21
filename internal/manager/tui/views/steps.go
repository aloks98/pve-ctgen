package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

type stepsMode int

const (
	stepsModeList stepsMode = iota
	stepsModeAdd
	stepsModeEdit
	stepsModeShowCmd
)

// StepsModel manages the build steps view.
type StepsModel struct {
	table     components.Table
	db        *store.DB
	steps     []models.BuildStep
	names     []string
	mode      stepsMode
	form      components.Form
	editName  string
	detail    string
	statusMsg string
	width     int
	height    int
}

// NewStepsModel creates a new StepsModel.
func NewStepsModel(db *store.DB) StepsModel {
	t := components.NewTable(
		[]string{"#", "NAME", "COMMAND"},
		[]int{5, 25, 60},
	)
	return StepsModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *StepsModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
}

// Refresh reloads data from the store.
func (m *StepsModel) Refresh() {
	steps, err := m.db.ListBuildSteps()
	if err != nil {
		return
	}
	m.steps = steps
	rows := make([][]string, len(steps))
	m.names = make([]string, len(steps))
	for i, s := range steps {
		cmd := s.Command
		if len(cmd) > 55 {
			cmd = cmd[:52] + "..."
		}
		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			s.Name,
			cmd,
		}
		m.names[i] = s.Name
	}
	m.table.SetRows(rows)
	m.mode = stepsModeList
	m.statusMsg = ""
}

// InSubView returns true if the view is in a sub-mode.
func (m *StepsModel) InSubView() bool {
	return m.mode != stepsModeList
}

func newStepForm(title string) components.Form {
	return components.NewForm(title, []components.FormField{
		{Label: "Name", Placeholder: "e.g. Resize disk", Required: true},
		{Label: "Command", Placeholder: "e.g. qemu-img resize ...", Required: true},
	})
}

// Update handles key events.
func (m StepsModel) Update(msg tea.Msg) (StepsModel, tea.Cmd) {
	switch m.mode {
	case stepsModeAdd, stepsModeEdit:
		return m.updateFormMode(msg)
	case stepsModeShowCmd:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "backspace" {
				m.mode = stepsModeList
			}
		}
		return m, nil
	}

	// List mode
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "a":
			m.form = newStepForm("Add Build Step")
			m.mode = stepsModeAdd
			return m, m.form.Fields[0].BlinkCmd()
		case "e":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.steps) {
				s := m.steps[idx]
				m.editName = s.Name
				m.form = newStepForm("Edit Build Step")
				m.form.SetValue("Name", s.Name)
				m.form.SetValue("Command", s.Command)
				m.mode = stepsModeEdit
				return m, m.form.Fields[0].BlinkCmd()
			}
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.steps) {
				s := m.steps[idx]
				m.detail = fmt.Sprintf("  Name:    %s\n  Order:   %d\n  Command: %s", s.Name, idx+1, s.Command)
				m.mode = stepsModeShowCmd
			}
		case "d":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.names) {
				name := m.names[idx]
				m.db.DeleteBuildStep(name)
				m.Refresh()
				m.statusMsg = fmt.Sprintf("Deleted %q", name)
			}
		case "K": // Move up (capital K)
			idx := m.table.SelectedRow()
			if idx > 0 && idx < len(m.steps) {
				a := m.steps[idx]
				if err := m.db.SwapBuildStepOrder(idx, idx-1); err == nil {
					cursor := m.table.Cursor
					m.Refresh()
					m.table.Cursor = cursor - 1
					m.statusMsg = fmt.Sprintf("Moved %q up", a.Name)
				}
			}
		case "J": // Move down (capital J)
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.steps)-1 {
				a := m.steps[idx]
				if err := m.db.SwapBuildStepOrder(idx, idx+1); err == nil {
					cursor := m.table.Cursor
					m.Refresh()
					m.table.Cursor = cursor + 1
					m.statusMsg = fmt.Sprintf("Moved %q down", a.Name)
				}
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

func (m StepsModel) updateFormMode(msg tea.Msg) (StepsModel, tea.Cmd) {
	cmd := m.form.Update(msg)
	if m.form.Cancelled {
		m.mode = stepsModeList
		return m, nil
	}
	if m.form.Submitted {
		name := m.form.Value("Name")
		command := m.form.Value("Command")

		if name == "" || command == "" {
			m.statusMsg = "Name and Command are required"
			m.form.Submitted = false
			return m, nil
		}

		if m.mode == stepsModeAdd {
			order := len(m.steps) + 1
			if _, err := m.db.CreateBuildStep(name, command, order); err != nil {
				m.statusMsg = fmt.Sprintf("Error: %v", err)
				m.form.Submitted = false
				return m, nil
			}
			m.Refresh()
			m.statusMsg = fmt.Sprintf("Added %q at position %d", name, order)
		} else {
			updates := map[string]interface{}{
				"name":    name,
				"command": command,
			}
			if err := m.db.UpdateBuildStep(m.editName, updates); err != nil {
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

// View renders the build steps.
func (m StepsModel) View() string {
	var b strings.Builder

	switch m.mode {
	case stepsModeAdd, stepsModeEdit:
		b.WriteString("\n")
		b.WriteString(m.form.View())
		if m.statusMsg != "" {
			b.WriteString("\n  " + styles.ErrorStyle.Render(m.statusMsg))
		}
		return b.String()

	case stepsModeShowCmd:
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Build Step Detail "))
		b.WriteString("\n\n")
		b.WriteString(m.detail)
		b.WriteString("\n\n")
		b.WriteString(styles.MutedStyle.Render("  esc: back"))
		return b.String()
	}

	// List mode
	b.WriteString("\n")
	b.WriteString(styles.TitleStyle.Render(" Build Steps "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.MutedStyle.Render("  enter: detail • a: add • e: edit • d: delete • J/K: reorder"))
	if m.statusMsg != "" {
		b.WriteString("  " + styles.SuccessStyle.Render(m.statusMsg))
	}
	return b.String()
}
