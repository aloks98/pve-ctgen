package tui

import (
	"strings"
	"testing"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/views"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testApp(t *testing.T) AppModel {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewApp(db, "test")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	return next.(AppModel)
}

func key(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

func TestWorkspaceFitsAndNavigation(t *testing.T) {
	m := testApp(t)
	for _, size := range [][2]int{{140, 42}, {100, 30}, {80, 24}, {40, 12}, {20, 5}, {1, 1}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(AppModel)
		for _, d := range destinations {
			next, _ = m.Update(NavigateMsg{View: d.view})
			m = next.(AppModel)
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size[1] {
				t.Fatalf("%s at %v: %d lines", d.label, size, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) != size[0] {
					t.Fatalf("%s at %v: line width %d", d.label, size, lipgloss.Width(line))
				}
			}
		}
	}
	m = testApp(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = next.(AppModel)
	next, _ = m.Update(key("2"))
	m = next.(AppModel)
	if m.currentView != ViewNodes || m.navFocused {
		t.Fatal("workspace navigation failed")
	}
	next, _ = m.Update(key("a"))
	m = next.(AppModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = next.(AppModel)
	if m.navFocused || !m.inSubView() {
		t.Fatal("navigation stole form focus")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestBackgroundBuildSurvivesNavigation(t *testing.T) {
	m := testApp(t)
	m.buildProgress.Reset(make(chan tea.Msg))
	next, _ := m.Update(NavigateMsg{View: ViewBuildSelect})
	m = next.(AppModel)
	if m.currentView != ViewBuildProgress {
		t.Fatal("allowed a second active build")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(AppModel)
	if m.currentView != ViewTemplates || !m.buildProgress.Running() {
		t.Fatal("lost background build")
	}
	next, cmd := m.Update(views.BuildStreamMsg{Message: views.BuildEventMsg{Event: &pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: "still running"}}})
	m = next.(AppModel)
	if cmd == nil || m.currentView != ViewTemplates {
		t.Fatal("background listener stopped or stole navigation")
	}
}

func TestStaleCancelCannotStopNewQueue(t *testing.T) {
	m := testApp(t)
	m.session = newBuildSession(models.Node{Name: "tokyo"})
	defer m.session.close()
	m.buildProgress.Reset(m.session.events)
	m.buildProgress, _ = m.buildProgress.Update(views.BuildStartedMsg{Request: &pb.BuildRequest{BuildId: "new-job", Image: &pb.Image{Name: "debian", Id: 9400}}})
	_, cmd := m.Update(views.CancelBuildMsg{ID: "old-job"})
	if cmd != nil || m.session.stopQueue.Load() {
		t.Fatal("stale cancellation affected new queue")
	}
}
