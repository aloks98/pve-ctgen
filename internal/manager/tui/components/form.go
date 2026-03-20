package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aloks98/pve-ctgen/internal/manager/tui/styles"
)

// FormField defines a single form field.
type FormField struct {
	Label       string
	Placeholder string
	Value       string
	Required    bool
	input       textinput.Model
}

// BlinkCmd returns a tea.Cmd to start the cursor blink on this field's input.
func (f *FormField) BlinkCmd() tea.Cmd {
	return f.input.Focus()
}

// Form is a multi-field form component with proper text input handling.
type Form struct {
	Title    string
	Fields   []FormField
	cursor   int
	focused  bool
	Submitted bool
	Cancelled bool
}

// NewForm creates a new Form with the given fields.
func NewForm(title string, fields []FormField) Form {
	for i := range fields {
		ti := textinput.New()
		ti.Placeholder = fields[i].Placeholder
		ti.SetValue(fields[i].Value)
		ti.CharLimit = 256
		ti.Width = 50
		if i == 0 {
			ti.Focus()
		}
		fields[i].input = ti
	}
	return Form{
		Title:  title,
		Fields: fields,
		focused: true,
	}
}

// Values returns the current field values as a map of label->value.
func (f *Form) Values() map[string]string {
	m := make(map[string]string, len(f.Fields))
	for _, field := range f.Fields {
		m[field.Label] = field.input.Value()
	}
	return m
}

// Value returns the value of a field by label.
func (f *Form) Value(label string) string {
	for _, field := range f.Fields {
		if field.Label == label {
			return field.input.Value()
		}
	}
	return ""
}

// SetValue sets the value of a field by label.
func (f *Form) SetValue(label, value string) {
	for i, field := range f.Fields {
		if field.Label == label {
			f.Fields[i].input.SetValue(value)
			return
		}
	}
}

// Reset clears the form state for reuse.
func (f *Form) Reset() {
	f.Submitted = false
	f.Cancelled = false
	f.cursor = 0
	for i := range f.Fields {
		f.Fields[i].input.SetValue(f.Fields[i].Value)
		if i == 0 {
			f.Fields[i].input.Focus()
		} else {
			f.Fields[i].input.Blur()
		}
	}
}

// Update handles key events.
func (f *Form) Update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "tab", "down":
			f.Fields[f.cursor].input.Blur()
			f.cursor = (f.cursor + 1) % len(f.Fields)
			return f.Fields[f.cursor].input.Focus()
		case "shift+tab", "up":
			f.Fields[f.cursor].input.Blur()
			f.cursor = (f.cursor - 1 + len(f.Fields)) % len(f.Fields)
			return f.Fields[f.cursor].input.Focus()
		case "enter":
			if f.cursor == len(f.Fields)-1 {
				f.Submitted = true
				return nil
			}
			// Move to next field
			f.Fields[f.cursor].input.Blur()
			f.cursor++
			return f.Fields[f.cursor].input.Focus()
		case "esc":
			f.Cancelled = true
			return nil
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	f.Fields[f.cursor].input, cmd = f.Fields[f.cursor].input.Update(msg)
	return cmd
}

// View renders the form.
func (f *Form) View() string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" %s ", f.Title)))
	b.WriteString("\n\n")

	for i, field := range f.Fields {
		label := field.Label
		if field.Required {
			label += " *"
		}

		cursor := "  "
		if i == f.cursor {
			cursor = styles.SelectedStyle.Render("> ")
		}

		b.WriteString(fmt.Sprintf("  %s%-18s %s\n", cursor, label+":", field.input.View()))
	}

	b.WriteString("\n")
	b.WriteString(styles.MutedStyle.Render("  tab/↑↓: navigate fields • enter: next/submit • esc: cancel"))
	b.WriteString("\n")

	return b.String()
}
