package styles

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"
)

func TestCanvasPaintsEveryCellAfterNestedResets(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile); lipgloss.SetHasDarkBackground(dark) })
	for _, tc := range []struct {
		name    string
		profile termenv.Profile
		dark    bool
	}{
		{"truecolor-dark", termenv.TrueColor, true},
		{"truecolor-light", termenv.TrueColor, false},
		{"256-color", termenv.ANSI256, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lipgloss.SetColorProfile(tc.profile)
			lipgloss.SetHasDarkBackground(tc.dark)
			content := BoldStyle.Render("Title") + " padding\n" + TableSelectedRowStyle.Render("Selected") + " tail\n" + "\x1b[48;2;40;50;60mcustom\x1b[49m default\n\x1b[0;31mred\x1b[m plain"
			output := PaintCanvas(Fit(content, 24, 6))
			if lipgloss.Width(output) != 24 || lipgloss.Height(output) != 6 {
				t.Fatal("canvas dimensions changed")
			}
			parser := ansi.NewParser()
			state := byte(0)
			var pen cellbuf.Style
			row, col := 0, 0
			for len(output) > 0 {
				seq, width, n, next := ansi.DecodeSequence(output, state, parser)
				if strings.HasPrefix(seq, "\x1b[") && parser.Command() == 'm' {
					cellbuf.ReadStyle(parser.Params(), &pen)
				}
				if seq == "\n" {
					row++
					col = 0
				}
				if width > 0 {
					if pen.Bg == nil {
						t.Fatalf("terminal default background at row %d, column %d", row, col)
					}
					if tc.profile == termenv.TrueColor {
						var want color.Color = Canvas
						if row == 1 && col < 8 {
							want = BgSelected
						}
						if row == 2 && col < 6 {
							want = color.RGBA{40, 50, 60, 255}
						}
						// Lip Gloss/termenv can round RGB components down by one.
						gotRGB := color.NRGBAModel.Convert(pen.Bg).(color.NRGBA)
						wantRGB := color.NRGBAModel.Convert(want).(color.NRGBA)
						close := func(a, b uint8) bool { return int(a)-int(b) >= -1 && int(a)-int(b) <= 1 }
						if !close(gotRGB.R, wantRGB.R) || !close(gotRGB.G, wantRGB.G) || !close(gotRGB.B, wantRGB.B) {
							t.Fatalf("wrong background at row %d column %d: %v", row, col, pen.Bg)
						}
					}
					if row == 0 && col >= 5 && pen.Attrs.Contains(cellbuf.BoldAttr) {
						t.Fatal("restoring canvas leaked bold text into padding")
					}
					col += width
				}
				output, state = output[n:], next
			}
			if pen.Bg != nil || pen.Fg != nil {
				t.Fatal("canvas colors leak past the frame")
			}
		})
	}
}
