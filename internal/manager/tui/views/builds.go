package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/components"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// BuildsModel manages the build history view.
type BuildsModel struct {
	table      components.Table
	db         *store.DB
	builds     []store.BuildWithDetails
	detail     string
	showDetail bool
	viewport   viewport.Model
	vpReady    bool
	width      int
	height     int
}

// NewBuildsModel creates a new BuildsModel.
func NewBuildsModel(db *store.DB) BuildsModel {
	t := components.NewTable(
		[]string{"BUILD_ID", "TEMPLATE", "NODE", "STATUS", "STARTED", "DURATION"},
		[]int{10, 14, 14, 11, 20, 12},
	)
	return BuildsModel{table: t, db: db}
}

// SetSize sets the view dimensions.
func (m *BuildsModel) SetSize(w, h int) {
	m.table.Width = w
	m.table.Height = h - 4
	m.width = w
	m.height = h
	vpHeight := h - 5
	if !m.vpReady {
		m.viewport = viewport.New(w, vpHeight)
		m.vpReady = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = vpHeight
	}
}

// InSubView returns true when detail is showing.
func (m *BuildsModel) InSubView() bool {
	return m.showDetail
}

// Refresh reloads data from the store.
func (m *BuildsModel) Refresh() {
	builds, err := m.db.ListBuildsWithDetails(50)
	if err != nil {
		return
	}
	m.builds = builds
	rows := make([][]string, len(builds))
	for i, b := range builds {
		short := b.BuildID
		if len(short) > 8 {
			short = short[:8]
		}
		started := "-"
		if b.StartedAt != nil {
			started = b.StartedAt.Format("01-02 15:04:05")
		}
		duration := "-"
		if b.StartedAt != nil && b.CompletedAt != nil {
			duration = b.CompletedAt.Sub(*b.StartedAt).Round(1e9).String()
		}
		rows[i] = []string{short, b.TemplateName, b.NodeName, b.Status, started, duration}
	}
	m.table.SetRows(rows)
	m.showDetail = false
}

// Update handles key events.
func (m BuildsModel) Update(msg tea.Msg) (BuildsModel, tea.Cmd) {
	if m.showDetail {
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "backspace" {
				m.showDetail = false
				return m, nil
			}
		}
		// Pass to viewport for scrolling
		if m.vpReady {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			idx := m.table.SelectedRow()
			if idx >= 0 && idx < len(m.builds) {
				m.detail = m.buildDetail(idx)
				if m.vpReady {
					m.viewport.SetContent(m.detail)
					m.viewport.GotoTop()
				}
				m.showDetail = true
			}
		default:
			m.table.Update(msg)
		}
	}
	return m, nil
}

func (m *BuildsModel) buildDetail(idx int) string {
	b := m.builds[idx]
	var detail strings.Builder

	fmt.Fprintf(&detail, "  Build ID:  %s\n", b.BuildID)
	fmt.Fprintf(&detail, "  Template:  %s\n", b.TemplateName)
	fmt.Fprintf(&detail, "  Node:      %s\n", b.NodeName)
	fmt.Fprintf(&detail, "  Status:    %s\n", b.Status)
	if b.StartedAt != nil {
		fmt.Fprintf(&detail, "  Started:   %s\n", b.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if b.CompletedAt != nil {
		fmt.Fprintf(&detail, "  Completed: %s\n", b.CompletedAt.Format("2006-01-02 15:04:05"))
	}
	if b.StartedAt != nil && b.CompletedAt != nil {
		fmt.Fprintf(&detail, "  Duration:  %s\n", b.CompletedAt.Sub(*b.StartedAt).Round(1e9))
	}

	results, err := m.db.GetBuildStepResults(b.BuildID)
	if err != nil || len(results) == 0 {
		detail.WriteString("\n  No step results recorded.\n")
		return detail.String()
	}

	detail.WriteString("\n  Steps:\n")
	for _, r := range results {
		icon := styles.StatusTag(r.Status)
		duration := ""
		if r.StartedAt != nil && r.CompletedAt != nil {
			duration = styles.MutedStyle.Render(fmt.Sprintf(" (%s)", r.CompletedAt.Sub(*r.StartedAt).Round(1e9)))
		}
		fmt.Fprintf(&detail, "    %s %s%s\n", icon, r.StepName, duration)

		if r.Log != "" {
			lines := strings.Split(strings.TrimSpace(r.Log), "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Fprintf(&detail, "      %s\n", styles.DimStyle.Render(line))
				}
			}
		}
	}

	return detail.String()
}

// View renders the build history.
func (m BuildsModel) View() string {
	var b strings.Builder

	if m.showDetail {
		b.WriteString("\n")
		title := styles.SectionStyle.Render(" Build Detail ")
		scrollPct := styles.DimStyle.Render(fmt.Sprintf(" %d%%", int(m.viewport.ScrollPercent()*100)))
		b.WriteString(title + scrollPct)
		b.WriteString("\n\n")
		if m.vpReady {
			b.WriteString(m.viewport.View())
		} else {
			b.WriteString(m.detail)
		}
		b.WriteString("\n")
		b.WriteString(styles.HelpBar(
			styles.HelpEntry("j/k", "scroll"),
			styles.HelpEntry("esc", "back"),
		))
		return b.String()
	}

	b.WriteString("\n")
	b.WriteString(styles.SectionStyle.Render(" Build History "))
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")
	b.WriteString(styles.HelpBar(
		styles.HelpEntry("enter", "details"),
	))
	return b.String()
}
