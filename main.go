package main

import (
	"github.com/aloks98/pve-ctgen/pkg/generator"
	"github.com/aloks98/pve-ctgen/pkg/ui"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	ui := ui.NewUI()
	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.StepView, 3, 1, false).
		AddItem(ui.CommandView, 3, 1, false).
		AddItem(ui.OutputView, 0, 1, true)

	layout := tview.NewFlex().
		AddItem(ui.StepsTree, 0, 1, true).AddItem(rightPanel, 0, 3, true)

	pages := tview.NewPages().
		AddPage("main", layout, true, true)

	doneChan := make(chan struct{})

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			select {
			case <-doneChan:
				ui.App.Stop()
			default:
				confirm := tview.NewModal().
					SetText("Are you sure you want to quit?").
					AddButtons([]string{"Quit", "Cancel"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						if buttonLabel == "Quit" {
							ui.App.Stop()
						}
						pages.RemovePage("confirm")
					})
				pages.AddPage("confirm", confirm, true, true)
			}

		}
		return event
	})

	go func() {
		generator.Run(ui)
		close(doneChan)
	}()

	if err := ui.App.SetRoot(pages, true).Run(); err != nil {
		panic(err)
	}
}
