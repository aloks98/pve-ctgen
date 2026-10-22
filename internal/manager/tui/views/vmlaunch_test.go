package views

import (
	"strings"
	"testing"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

func TestVMLaunchKeepsLastFieldVisible(t *testing.T) {
	m := NewVMLaunchModel(testDB(t))
	m.phase = vmPhaseConfig
	m.selectedNode = &models.Node{Name: "tokyo"}
	m.selectedTmpl = &remoteTemplate{vmID: 9400, name: "debian"}
	m.SetSize(38, 20)
	m.cursor = len(m.visibleFields()) - 1
	if !strings.Contains(m.View(), "Overwrite if exists") {
		t.Fatal("last launch field scrolled out of view")
	}
}
