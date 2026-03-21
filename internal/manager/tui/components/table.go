package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// Table is a navigable table with header separator.
type Table struct {
	Headers   []string
	Rows      [][]string
	Cursor    int
	Width     int
	Height    int
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
			if len(t.Rows) > 0 {
				t.Cursor = len(t.Rows) - 1
			}
		}
	}
}

// View renders the table.
func (t *Table) View() string {
	if len(t.Headers) == 0 {
		return ""
	}

	var b strings.Builder

	// Header row
	var headerParts []string
	for i, h := range t.Headers {
		w := t.colWidth(i)
		headerParts = append(headerParts, styles.TableHeaderStyle.Width(w).Render(h))
	}
	b.WriteString(strings.Join(headerParts, styles.MutedStyle.Render(" ")))
	b.WriteString("\n")

	// Separator
	var sepParts []string
	for i := range t.Headers {
		w := t.colWidth(i)
		sepParts = append(sepParts, styles.MutedStyle.Render(strings.Repeat("-", w)))
	}
	b.WriteString(strings.Join(sepParts, styles.MutedStyle.Render("-")))
	b.WriteString("\n")

	// Empty state
	if len(t.Rows) == 0 {
		b.WriteString(styles.DimStyle.Render(" (empty)"))
		return b.String()
	}

	// Visible rows with scrolling
	visibleRows := t.Height - 3
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
		var cells []string
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
			cells = append(cells, style.Render(val))
		}
		b.WriteString(strings.Join(cells, " "))
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(t.Rows) > visibleRows {
		b.WriteString(styles.DimStyle.Render(fmt.Sprintf(" %d/%d", t.Cursor+1, len(t.Rows))))
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
