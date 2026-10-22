package views

import (
	"strings"
	"testing"

	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	tea "github.com/charmbracelet/bubbletea"
)

func TestProgressRetainsLogsAndDeduplicatesReplay(t *testing.T) {
	m := NewBuildProgressModel()
	m.SetSize(100, 30)
	m.Reset(make(chan tea.Msg))
	m, _ = m.Update(BuildStartedMsg{Request: &pb.BuildRequest{BuildId: "job", Image: &pb.Image{Name: "debian", Id: 9400}}, Node: "tokyo"})
	for _, event := range []*pb.BuildEvent{
		{BuildId: "job", Sequence: 1, Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: "retained diagnostic"},
		{BuildId: "job", Sequence: 2, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED},
		{BuildId: "job", Sequence: 2, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED},
	} {
		m, _ = m.Update(BuildEventMsg{Event: event})
	}
	m, _ = m.Update(BuildDoneMsg{})
	if m.succeeded != 1 || m.Running() || !strings.Contains(m.View(), "retained diagnostic") {
		t.Fatal("lost final logs or counted replay twice")
	}
}

func TestProgressCancellationNeedsConfirmation(t *testing.T) {
	m := NewBuildProgressModel()
	m.Reset(make(chan tea.Msg))
	m.id = "job"
	m, cmd := m.Update(press("x"))
	if cmd != nil || !m.InSubView() {
		t.Fatal("cancel executed before confirmation")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.InSubView() {
		t.Fatal("escape did not dismiss confirmation")
	}
	m, _ = m.Update(press("x"))
	m, cmd = m.Update(press("y"))
	if cmd == nil || cmd().(CancelBuildMsg).ID != "job" || !m.cancelling {
		t.Fatal("confirmed cancellation failed")
	}
}
