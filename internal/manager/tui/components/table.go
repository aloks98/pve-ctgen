package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// Table is a navigable table with header separator.
type Table struct {
	Headers      []string
	Rows         [][]string
	Cursor       int
	Width        int
	Height       int
	ColWidths    []int
	EmptyMessage string
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
		case "pgdown":
			t.Cursor = min(max(0, len(t.Rows)-1), t.Cursor+max(1, t.Height-3))
		case "pgup":
			t.Cursor = max(0, t.Cursor-max(1, t.Height-3))
		case "home", "g":
			t.Cursor = 0
		case "end", "G":
			if len(t.Rows) > 0 {
				t.Cursor = len(t.Rows) - 1
			}
		}
	}
}

// View renders a bounded table with display-cell-safe columns and a visible cursor.
func (t *Table) View() string {
	if len(t.Headers) == 0 {
		return ""
	}
	width := t.Width
	if width <= 0 {
		width = 80
	}
	columns := min(len(t.Headers), max(1, width/5))
	budget := max(columns, width-columns+1)
	widths := make([]int, columns)
	for i := range widths {
		widths[i] = max(3, t.colWidth(i))
	}
	sum := func() int {
		n := 0
		for _, w := range widths {
			n += w
		}
		return n
	}
	for sum() > budget {
		largest := 0
		for i := range widths {
			if widths[i] > widths[largest] {
				largest = i
			}
		}
		if widths[largest] <= 1 {
			break
		}
		widths[largest]--
	}
	// Distribute spare space proportionally instead of assigning it all to
	// the first column (which may be a short ID).
	total := sum()
	spare := max(0, budget-total)
	used := 0
	for i := range widths {
		extra := spare * widths[i] / total
		widths[i] += extra
		used += extra
	}
	for i := 0; i < spare-used; i++ {
		widths[i%columns]++
	}
	renderRow := func(row []string, selected, header bool) string {
		cells := make([]string, columns)
		for i := range cells {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			if i == 0 {
				if selected {
					value = "› " + value
				} else {
					value = "  " + value
				}
			}
			cells[i] = styles.Fit(value, widths[i], 1)
		}
		style := styles.TableRowStyle
		if header {
			style = styles.TableHeaderStyle
		} else if selected {
			style = styles.TableSelectedRowStyle
		}
		// Apply the selection to the whole row, including column gutters.
		return style.Render(strings.Join(cells, " "))
	}
	lines := []string{renderRow(t.Headers, false, true), styles.Rule(width)}
	if len(t.Rows) == 0 {
		message := t.EmptyMessage
		if message == "" {
			message = "No items found."
		}
		return strings.Join(append(lines, styles.DimStyle.Render(styles.Fit(message, width, 1))), "\n")
	}
	visible := max(1, t.Height-3)
	start := max(0, t.Cursor-visible+1)
	for i := start; i < min(len(t.Rows), start+visible); i++ {
		lines = append(lines, renderRow(t.Rows[i], i == t.Cursor, false))
	}
	if len(t.Rows) > visible {
		lines = append(lines, styles.DimStyle.Render(fmt.Sprintf("%d / %d  pgup/pgdown", t.Cursor+1, len(t.Rows))))
	}
	return strings.Join(lines, "\n")
}

func (t *Table) colWidth(i int) int {
	if i < len(t.ColWidths) {
		return t.ColWidths[i]
	}
	return 20
}
