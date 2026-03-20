package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// BuildsModel manages the build history view.
type BuildsModel struct {
	table    components.Table
	db       *store.DB
	buildIDs []string
	detail   string
	showDetail bool
}

// NewBuildsModel creates a new BuildsModel.
func NewBuildsModel(db *store.DB) BuildsModel {
	t := components.NewTable(
		[]string{"BUILD_ID", "STATUS", "STARTED", "DURATION"},
		[]int{12, 12, 22, 15},
	)
	return BuildsModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *BuildsModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h
}

// Refresh reloads data from the store.
func (m *BuildsModel) Refresh() {
	builds, err := m.db.ListBuilds("", "", "", 50)
	if err != nil {
		return
	}
	rows := make([][]string, len(builds))
	m.buildIDs = make([]string, len(builds))
	for i, b := range builds {
		short := b.BuildID
		if len(short) > 8 {
			short = short[:8]
		}
		started := "-"
		if b.StartedAt != nil {
			started = b.StartedAt.Format("2006-01-02 15:04:05")
		}
		duration := "-"
		if b.StartedAt != nil && b.CompletedAt != nil {
			duration = b.CompletedAt.Sub(*b.StartedAt).Round(1e9).String()
		}
		rows[i] = []string{short, b.Status, started, duration}
		m.buildIDs[i] = b.BuildID
	}
	m.table.SetRows(rows)
	m.showDetail = false
}

// Update handles key events.
func (m BuildsModel) Update(msg tea.Msg) (BuildsModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if m.showDetail {
			if km.String() == "esc" || km.String() == "backspace" {
				m.showDetail = false
			}
			return m, nil
		}

		switch km.String() {
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.buildIDs) {
				results, err := m.db.GetBuildStepResults(m.buildIDs[idx])
				if err == nil {
					detail := fmt.Sprintf("Build: %s\n\n", m.buildIDs[idx])
					for _, r := range results {
						icon := styles.StatusIcon(r.Status)
						detail += fmt.Sprintf("  %s %s\n", icon, r.StepName)
						if r.Log != "" {
							detail += fmt.Sprintf("    %s\n", r.Log)
						}
					}
					m.detail = detail
					m.showDetail = true
				}
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

// View renders the build history.
func (m BuildsModel) View() string {
	if m.showDetail {
		return fmt.Sprintf("\n  Build Detail (press esc to go back)\n\n%s", m.detail)
	}
	return fmt.Sprintf("\n  Build History (enter: details)\n\n%s", m.table.View())
}
