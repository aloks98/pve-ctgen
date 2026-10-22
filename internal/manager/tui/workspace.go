package tui

import (
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

// Keep large terminals readable without stretching short IDs across the screen.
func (m AppModel) shellWidth() int { return min(156, max(1, m.width-4)) }

func (m AppModel) contentSize() (int, int) {
	w := m.shellWidth()
	if m.width >= 100 {
		w -= 24
	}
	h := m.height - 4
	if m.height >= 24 {
		h -= 3
	}
	return max(1, w), max(1, h)
}

func (m *AppModel) resizeViews() {
	w, h := m.contentSize()
	m.cloudinitView.SetSize(w, h)
	m.templatesView.SetSize(w, h)
	m.stepsView.SetSize(w, h)
	m.buildsView.SetSize(w, h)
	m.buildSelect.SetSize(min(w, 88), h)
	m.buildProgress.SetSize(w, h)
	m.vmLaunch.SetSize(w, h)
	m.nodesView.SetSize(w, h)
}

func (m AppModel) navigation(height int) string {
	lines := []string{styles.DimStyle.Render(" WORKSPACE"), ""}
	visible := max(1, height-4)
	cursor := m.navCursor
	if !m.navFocused {
		for i, d := range destinations {
			if d.view == m.currentView {
				cursor = i
			}
		}
	}
	start := max(0, cursor-visible+1)
	for i := start; i < min(len(destinations), start+visible); i++ {
		d := destinations[i]
		label := "  " + d.label
		style := styles.MenuItemStyle
		if m.currentView == d.view {
			label = "› " + d.label
			style = styles.TableSelectedRowStyle
		}
		if m.navFocused && m.navCursor == i {
			label = "› " + d.label
			style = styles.TableSelectedRowStyle.Underline(true)
		}
		lines = append(lines, style.Render(styles.Fit(label, 20, 1)))
	}
	if height >= len(lines)+2 {
		for len(lines) < height-2 {
			lines = append(lines, "")
		}
		if m.buildProgress.Running() {
			lines = append(lines, styles.WarningStyle.Render(" [..] Build active"))
		} else {
			lines = append(lines, styles.DimStyle.Render(" No active build"))
		}
	}
	return styles.Fit(strings.Join(lines, "\n"), 20, height)
}

func (m AppModel) navigationKey() string {
	if m.macKeys {
		return "cmd+g"
	}
	return "ctrl+g"
}

func (m AppModel) workspace(content string) string {
	if m.width < 40 || m.height < 12 {
		return styles.Fit("pve-ctgen\nResize terminal to at least 40 x 12.\nctrl+c quit", m.width, m.height)
	}
	w, h := m.contentSize()
	shell := m.shellWidth()
	title := styles.BoldStyle.Render("pve-ctgen") + "   " + styles.DimStyle.Render("workspace / ") + styles.AccentStyle.Render(strings.TrimPrefix(m.breadcrumb(), "pvectgen > "))
	if m.buildProgress.Running() {
		title += "  " + styles.WarningStyle.Render("[..] build active")
	}
	if m.navFocused && m.width < 100 {
		content = m.navigation(h)
	}
	body := styles.Fit(content, w, h)
	if m.currentView == ViewBuildSelect && !(m.navFocused && m.width < 100) && w > 88 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, strings.Repeat(" ", (w-88)/2), styles.Fit(content, 88, h))
	}
	if m.width >= 100 {
		divider := styles.RuleStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", h), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.navigation(h), " ", divider, "  ", body)
	}
	footer := styles.HelpEntry(m.navigationKey(), "workspace") + "  " + m.helpKeys()
	if m.navFocused {
		footer = styles.HelpBar(styles.HelpEntry(m.navigationKey(), "close"), styles.HelpEntry("j/k", "move"), styles.HelpEntry("enter", "open"), styles.HelpEntry("1–9", "jump"))
	}
	lines := []string{styles.Fit(title, shell, 1), styles.Rule(shell)}
	if m.height >= 24 {
		lines = append([]string{""}, lines...)
		lines = append(lines, "")
	}
	lines = append(lines, body, styles.Rule(shell), styles.Fit(footer, shell, 1))
	if m.height >= 24 {
		lines = append(lines, "")
	}
	// Center the bounded workspace on very wide terminals; retain a two-cell
	// margin at ordinary widths. Paint all blank cells, not just text fragments.
	margin := strings.Repeat(" ", max(0, (m.width-shell)/2))
	output := strings.Split(strings.Join(lines, "\n"), "\n")
	for i := range output {
		output[i] = margin + output[i]
	}
	return styles.CanvasStyle.Render(styles.Fit(strings.Join(output, "\n"), m.width, m.height))
}
