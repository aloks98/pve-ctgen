package views

import (
	"context"
	"strings"
	"testing"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	tea "github.com/charmbracelet/bubbletea"
)

func reviewModel(t *testing.T) BuildSelectModel {
	t.Helper()
	db := testDB(t)
	if _, err := db.CreateTemplate(&models.Template{Name: "debian", VMID: 9400, URL: "https://example.com/disk.img", ChecksumURL: "https://example.com/SHA256SUMS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNode("tokyo", "", "localhost:50051", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateBuildStep("Configure", "echo original", 0); err != nil {
		t.Fatal(err)
	}
	m := NewBuildSelectModel(db)
	m.SetSize(76, 24)
	m.preflight = func(_ models.Node, _ []*pb.BuildRequest, _ uint64, _ context.Context) tea.Cmd { return nil }
	m.SelectTemplate("debian")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // node -> VM IDs
	return m
}

func TestBuildRequiresSuccessfulReviewAndExplicitConfirmation(t *testing.T) {
	m := reviewModel(t)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != phaseReview || !m.checking || cmd != nil {
		t.Fatal("did not enter read-only review")
	}
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("started before checks completed")
	}
	m, _ = m.Update(preflightResultMsg{revision: m.revision, checks: []*pb.CheckResult{{Name: "storage", Ok: false}}})
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("failed check allowed start")
	}
	m, _ = m.Update(press("r"))
	if !m.checking || m.ready {
		t.Fatal("retry did not reset readiness")
	}
	m, _ = m.Update(preflightResultMsg{revision: m.revision - 1, checks: []*pb.CheckResult{{Ok: true}}})
	if m.ready {
		t.Fatal("stale result enabled start")
	}
	m, _ = m.Update(preflightResultMsg{revision: m.revision, checks: []*pb.CheckResult{{Ok: true}}})
	if !m.ready {
		t.Fatal("successful checks did not enable confirmation")
	}
	if err := m.db.UpdateBuildStep("Configure", map[string]interface{}{"command": "echo changed"}); err != nil {
		t.Fatal(err)
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no start after confirmation")
	}
	start := cmd().(StartBuildMsg)
	if start.Requests[0].Steps[0].Command != "echo original" {
		t.Fatal("approved request changed before submit")
	}
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("double submit")
	}
	builds, err := m.db.ListBuilds("", "", "", 0)
	if err != nil || len(builds) != 0 {
		t.Fatal("review created a job/history entry")
	}
}

func TestBuildReviewRejectsBadIDsAndIgnoresAbandonedChecks(t *testing.T) {
	m := reviewModel(t)
	m.form.SetValue("debian", "4294976696")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.phase != phaseVMIDs || !strings.Contains(m.statusMsg, "VM IDs") {
		t.Fatal("out-of-range ID accepted")
	}
	m.form.SetValue("debian", "9400")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	revision := m.revision
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(preflightResultMsg{revision: revision, checks: []*pb.CheckResult{{Ok: true}}})
	if m.phase != phaseVMIDs || m.ready {
		t.Fatal("abandoned checks changed state")
	}
}
