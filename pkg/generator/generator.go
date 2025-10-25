package generator

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/pkg/style"
	"github.com/aloks98/pve-ctgen/pkg/ui"
	"github.com/aloks98/pve-ctgen/pkg/utils"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Run is the main function for the generator.
func Run(ui *ui.UI) {
	isoFilePath := "/var/lib/vz/template/iso"
	snippetsFilePath := "/var/lib/vz/snippets"

	images, err := utils.LoadImages("config/os_list.json")
	if err != nil {
		ui.ShowErrorModal(fmt.Sprintf("Error loading images: %v", err))
		return
	}
	steps, err := utils.LoadSteps("config/steps.json")
	if err != nil {
		ui.ShowErrorModal(fmt.Sprintf("Error loading steps: %v", err))
		return
	}

	uiImages := ui.BuildUITree(images, steps)

	if err := os.MkdirAll(isoFilePath, 0755); err != nil {
		ui.ShowErrorModal(fmt.Sprintf("Error creating iso folder: %v. Do you have proper permissions?", err))
		return
	}
	if err := os.MkdirAll(snippetsFilePath, 0755); err != nil {
		ui.ShowErrorModal(fmt.Sprintf("Error creating snippets folder: %v. Do you have proper permissions?", err))
		return
	}
	if err := os.MkdirAll("logs", 0755); err != nil {
		ui.ShowErrorModal(fmt.Sprintf("Error creating logs folder: %v", err))
		return
	}

	var previousImageNode *tview.TreeNode
	var failedImages []string
	for _, uiImage := range uiImages {
		ui.App.QueueUpdateDraw(func() {
			if previousImageNode != nil {
				previousImageNode.Collapse()
			}
			uiImage.Node.SetColor(tcell.ColorYellow)
			ui.StepsTree.SetCurrentNode(uiImage.Node)
			uiImage.Node.Expand()
			previousImageNode = uiImage.Node
		})

		var hasFailed bool
		var filePath string

		// --- Download & Verify Step ---
		downloadStep := uiImage.Steps[0]
		filePath, err = utils.HandleDownloadAndChecksum(ui.App, ui.StepView, ui.CommandView, ui.OutputView, downloadStep, uiImage.Image, isoFilePath, ui.UpdateNodeStatus, ui.AppendOutput)
		if err != nil {
			utils.LogError(uiImage.Image.Name, err)
			ui.UpdateNodeStatus(downloadStep.Node, "failed")
			hasFailed = true
		} else {
			ui.UpdateNodeStatus(downloadStep.Node, "success")
			time.Sleep(1 * time.Second)
		}

		// --- Copy Image Step ---
		baseFilePath := "base.qcow2"
		if !hasFailed {
			copyStep := uiImage.Steps[1]
			ui.UpdateNodeStatus(copyStep.Node, "running")
			ui.App.QueueUpdateDraw(func() {
				ui.StepView.Clear()
				ui.CommandView.Clear()
				ui.OutputView.Clear()
				ui.StepView.SetText(copyStep.Name)
				writer := tview.ANSIWriter(ui.CommandView)
				fmt.Fprint(writer, style.Yellow(fmt.Sprintf("cp %s %s", filePath, baseFilePath)))
			})
			ui.AppendOutput(fmt.Sprintf("Copying %s to %s...\n", filePath, baseFilePath))
			if err := utils.CopyFile(filePath, baseFilePath); err != nil {
				utils.LogError(uiImage.Image.Name, err)
				ui.UpdateNodeStatus(copyStep.Node, "failed")
				hasFailed = true
			} else {
				ui.AppendOutput("Copy complete.\n")
				ui.UpdateNodeStatus(copyStep.Node, "success")
				time.Sleep(1 * time.Second)
			}
		}

		// --- Dynamic Execution Steps ---
		if !hasFailed {
			executionSteps := uiImage.Steps[2:]
			if err := utils.ExecuteCommands(ui.App, ui.StepView, ui.CommandView, ui.OutputView, executionSteps, baseFilePath, uiImage.Image, steps, ui.UpdateNodeStatus, ui.AppendOutput, utils.LogError); err != nil {
				utils.LogError(uiImage.Image.Name, err)
				hasFailed = true
			}
		}

		// --- Final Status ---
		if hasFailed {
			failedImages = append(failedImages, uiImage.Image.Name)
			ui.MarkRemainingStepsAsSkipped(uiImage.Steps)
			uiImage.Node.SetText(fmt.Sprintf("❌ %s", uiImage.Image.Name))
		} else {
			uiImage.Node.SetText(fmt.Sprintf("✅ %s", uiImage.Image.Name))
		}
		uiImage.Node.SetColor(tcell.ColorDefault)
	}

	var finalMessage strings.Builder
	if len(failedImages) == 0 {
		finalMessage.WriteString(style.Green("\nAll steps completed successfully!\n"))
	} else {
		finalMessage.WriteString(style.Red("\nThe following images failed:\n"))
		for _, imgName := range failedImages {
			finalMessage.WriteString(fmt.Sprintf("- %s\n", imgName))
		}
	}
	finalMessage.WriteString(style.Yellow("\nPress ESC to exit."))

	writer := tview.ANSIWriter(ui.OutputView)
	fmt.Fprint(writer, finalMessage.String())
}
