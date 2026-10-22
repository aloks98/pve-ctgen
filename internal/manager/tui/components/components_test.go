package components

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestWideTableKeepsColumnsTogether(t *testing.T) {
	table := NewTable([]string{"BUILD", "TEMPLATE", "NODE", "STATUS", "STARTED", "DURATION"}, []int{10, 14, 14, 11, 20, 12})
	table.Width, table.Height = 200, 12
	table.SetRows([][]string{{"12345678", "debian-12", "tokyo", "completed", "10-23 12:00:00", "2m"}})
	lines := strings.Split(ansi.Strip(table.View()), "\n")
	if pos := strings.Index(lines[0], "TEMPLATE"); pos < 0 || pos > 35 {
		t.Fatalf("short build ID consumed the wide table: %q", lines[0])
	}
	prefix := strings.Split(lines[2], "12345678")[0]
	if lipgloss.Width(prefix) != strings.Index(lines[0], "BUILD") {
		t.Fatal("first column header and value are misaligned")
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 200 {
			t.Fatalf("table row does not fill its width: %d", lipgloss.Width(line))
		}
	}
}

func TestTableUnicodeAndPaging(t *testing.T) {
	table := NewTable([]string{"NAME", "ID", "PROFILE"}, []int{28, 8, 20})
	table.Width = 30
	table.Height = 7
	for i := 0; i < 50; i++ {
		table.Rows = append(table.Rows, []string{fmt.Sprintf("日本語-%d-very-long", i), fmt.Sprint(i), "default"})
	}
	table.Update(tea.KeyMsg{Type: tea.KeyEnd})
	view := table.View()
	if !utf8.ValidString(view) || !strings.Contains(view, "49") {
		t.Fatal("last Unicode row missing or corrupt")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("table overflow: %q", line)
		}
	}
	if lipgloss.Height(view) > 7 {
		t.Fatal("table too tall")
	}
	table.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if table.Cursor != 45 {
		t.Fatalf("page up cursor: %d", table.Cursor)
	}
}

func TestFormKeepsFocusedFieldVisible(t *testing.T) {
	var fields []FormField
	for i := 0; i < 12; i++ {
		fields = append(fields, FormField{Label: fmt.Sprintf("Field-%02d", i)})
	}
	f := NewForm("Edit", fields)
	f.SetSize(38, 12)
	for i := 0; i < 11; i++ {
		f.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if !strings.Contains(f.View(), "Field-11") || strings.Contains(f.View(), "Field-00") {
		t.Fatal("focused field scrolled out")
	}
	f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("value")})
	if f.Value("Field-11") != "value" {
		t.Fatal("wrong field edited")
	}
	if lipgloss.Height(f.View()) > 12 {
		t.Fatal("form too tall")
	}
}

func TestLogsPauseFollowAndBoundBuffer(t *testing.T) {
	l := NewLogViewer("Output")
	l.SetSize(40, 5)
	for i := 0; i < 20; i++ {
		l.AppendLine(fmt.Sprintf("line %d", i))
	}
	l.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	before := l.viewport.YOffset
	l.AppendLine("new message")
	if l.follow || l.viewport.YOffset != before {
		t.Fatal("new log stole scroll position")
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if !strings.Contains(l.View(), "new message") {
		t.Fatal("follow did not resume")
	}
	l.lines = make([]string, 5000)
	l.AppendLine("last message")
	if len(l.lines) != 5000 {
		t.Fatal("live log buffer unbounded")
	}
}
