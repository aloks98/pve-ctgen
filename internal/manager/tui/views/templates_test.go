package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTemplateDetailsAndActionsFitWithLongList(t *testing.T) {
	db := testDB(t)
	for i := 0; i < 30; i++ {
		_, err := db.CreateTemplate(&models.Template{Name: fmt.Sprintf("template-%02d", i), VMID: 9400 + i, URL: "https://example.invalid/disk.img", ChecksumURL: "https://example.invalid/SHA256SUMS"})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := NewTemplatesModel(db)
	m.Refresh()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	for _, size := range [][2]int{{132, 51}, {92, 29}, {76, 17}} {
		m.SetSize(size[0], size[1])
		view := m.View()
		if lipgloss.Height(view) > size[1] || !strings.Contains(view, "template-29") || !strings.Contains(view, "enter details") {
			t.Fatalf("selected row or actions overflow at %v: height %d", size, lipgloss.Height(view))
		}
	}
}

func testDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func press(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

func TestTemplateFilterBuildAndDelete(t *testing.T) {
	db := testDB(t)
	for i, name := range []string{"ubuntu", "debian"} {
		_, err := db.CreateTemplate(&models.Template{Name: name, VMID: 9400 + i, URL: "https://example.com/disk.img", ChecksumURL: "https://example.com/SHA256SUMS", Profile: "lab"})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := NewTemplatesModel(db)
	m.SetSize(76, 30)
	m.Refresh()
	m, _ = m.Update(press("/"))
	m, _ = m.Update(press("deb"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.names) != 1 || m.names[0] != "debian" {
		t.Fatal("filter did not select Debian")
	}
	_, cmd := m.Update(press("b"))
	if cmd == nil || cmd().(BuildTemplateMsg).Name != "debian" {
		t.Fatal("build targets wrong template")
	}
	m, _ = m.Update(press("d"))
	if _, err := db.GetTemplate("debian"); err != nil {
		t.Fatal("deleted before confirmation")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(press("e"))
	if m.form.Value("Profile") != "lab" {
		t.Fatal("edit lost profile")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(press("d"))
	m, _ = m.Update(press("y"))
	if _, err := db.GetTemplate("debian"); err == nil {
		t.Fatal("confirmed delete failed")
	}
	if _, err := db.GetTemplate("ubuntu"); err != nil {
		t.Fatal("deleted unrelated template")
	}
}
