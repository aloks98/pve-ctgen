package tui

import (
	"fmt"
	"strings"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

var destinations = []struct {
	label string
	view  View
}{
	{"Templates", ViewTemplates},
	{"Nodes", ViewNodes},
	{"Init configs", ViewCloudInit},
	{"Build steps", ViewSteps},
	{"History", ViewBuilds},
	{"New build", ViewBuildSelect},
	{"Launch VM", ViewVMLaunch},
	{"Build progress", ViewBuildProgress},
	{"Overview", ViewHome},
}

func (m AppModel) contentSize() (int, int) {
	w := m.width - 2
	if m.width >= 100 {
		w -= 22
	}
	return max(1, w), max(1, m.height-4)
}

func (m *AppModel) resizeViews() {
	w, h := m.contentSize()
	m.cloudinitView.SetSize(w, h)
	m.templatesView.SetSize(w, h)
	m.stepsView.SetSize(w, h)
	m.buildsView.SetSize(w, h)
	m.buildSelect.SetSize(w, h)
	m.buildProgress.SetSize(w, h)
	m.vmLaunch.SetSize(w, h)
	m.nodesView.SetSize(w, h)
}

func (m AppModel) navigation(height int) string {
	lines := []string{styles.DimStyle.Render("WORKSPACE"), ""}
	start := 0
	if m.navFocused {
		start = max(0, m.navCursor-max(1, height-4)+1)
	}
	for i, d := range destinations {
		if i < start || i >= start+max(1, height-4) {
			continue
		}
		label := fmt.Sprintf(" %d %s", i+1, d.label)
		style := styles.MenuItemStyle
		if m.currentView == d.view {
			label = ">" + label[1:]
			style = styles.SelectedStyle
		}
		if m.navFocused && m.navCursor == i {
			style = styles.TableSelectedRowStyle
		}
		lines = append(lines, style.Render(label))
	}
	if m.buildProgress.Running() {
		lines = append(lines, "", styles.WarningStyle.Render("[..] Build active"))
	}
	if m.navFocused {
		lines = append(lines, "", styles.DimStyle.Render("j/k move  enter open"))
	}
	return styles.Fit(strings.Join(lines, "\n"), 20, height)
}

func (m AppModel) workspace(content string) string {
	if m.width < 40 || m.height < 12 {
		return styles.Fit("pve-ctgen\nResize terminal to at least 40 x 12.\nctrl+c quit", m.width, m.height)
	}
	w, h := m.contentSize()
	title := styles.BoldStyle.Render("pve-ctgen") + "  /  " + styles.AccentStyle.Render(strings.TrimPrefix(m.breadcrumb(), "pvectgen > "))
	if m.buildProgress.Running() {
		title += "  " + styles.WarningStyle.Render("[..] build active")
	}
	if m.navFocused && m.width < 100 {
		content = m.navigation(h)
	}
	body := styles.Fit(content, w, h)
	if m.width >= 100 {
		divider := styles.MutedStyle.Render(strings.TrimSuffix(strings.Repeat("|\n", h), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.navigation(h), divider, " ", body)
	}
	footer := "ctrl+g workspace  " + m.helpKeys()
	if m.navFocused {
		footer = "ctrl+g close  j/k move  enter open"
	}
	return styles.Fit(title, m.width, 1) + "\n" + styles.Fit(styles.MutedStyle.Render(strings.Repeat("-", m.width)), m.width, 1) + "\n" + styles.Fit(body, m.width, h) + "\n" + styles.Fit("", m.width, 1) + "\n" + styles.Fit(footer, m.width, 1)
}
