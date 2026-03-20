package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// Table is a simple navigable table component.
type Table struct {
	Headers  []string
	Rows     [][]string
	Cursor   int
	Width    int
	Height   int
	ColWidths []int
}

// NewTable creates a new Table.
func NewTable(headers []string, colWidths []int) Table {
	return Table{
		Headers:   headers,
		ColWidths: colWidths,
	}
}

// SetRows replaces the table data.
func (t *Table) SetRows(rows [][]string) {
	t.Rows = rows
	if t.Cursor >= len(rows) {
		t.Cursor = max(0, len(rows)-1)
	}
}

// SelectedRow returns the currently selected row index, or -1 if empty.
func (t *Table) SelectedRow() int {
	if len(t.Rows) == 0 {
		return -1
	}
	return t.Cursor
}

// Update handles key events.
func (t *Table) Update(msg tea.Msg) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if t.Cursor > 0 {
				t.Cursor--
			}
		case "down", "j":
			if t.Cursor < len(t.Rows)-1 {
				t.Cursor++
			}
		case "home", "g":
			t.Cursor = 0
		case "end", "G":
			t.Cursor = len(t.Rows) - 1
		}
	}
}

// View renders the table.
func (t *Table) View() string {
	if len(t.Headers) == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	headerCells := make([]string, len(t.Headers))
	for i, h := range t.Headers {
		w := t.colWidth(i)
		headerCells[i] = styles.TableHeaderStyle.Width(w).Render(h)
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	b.WriteString("\n")

	// Rows
	if len(t.Rows) == 0 {
		b.WriteString(styles.MutedStyle.Render("  (empty)"))
		return b.String()
	}

	visibleRows := t.Height - 2
	if visibleRows <= 0 {
		visibleRows = len(t.Rows)
	}

	start := 0
	if t.Cursor >= visibleRows {
		start = t.Cursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(t.Rows) {
		end = len(t.Rows)
	}

	for i := start; i < end; i++ {
		row := t.Rows[i]
		cells := make([]string, len(t.Headers))
		for j := range t.Headers {
			w := t.colWidth(j)
			val := ""
			if j < len(row) {
				val = row[j]
			}
			if len(val) > w-1 && w > 4 {
				val = val[:w-4] + "..."
			}
			style := styles.TableRowStyle.Width(w)
			if i == t.Cursor {
				style = styles.TableSelectedRowStyle.Width(w)
			}
			cells[j] = style.Render(val)
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		b.WriteString("\n")
	}

	// Footer with position
	if len(t.Rows) > visibleRows {
		b.WriteString(styles.MutedStyle.Render(fmt.Sprintf("  %d/%d", t.Cursor+1, len(t.Rows))))
	}

	return b.String()
}

func (t *Table) colWidth(i int) int {
	if i < len(t.ColWidths) {
		return t.ColWidths[i]
	}
	if t.Width > 0 && len(t.Headers) > 0 {
		return t.Width / len(t.Headers)
	}
	return 20
}
