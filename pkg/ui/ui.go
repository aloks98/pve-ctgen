package ui

import (
	"fmt"
	"github.com/aloks98/pve-ctgen/pkg/types"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// UI holds all the UI components.
type UI struct {
	App        *tview.Application
	StepsTree  *tview.TreeView
	StepView   *tview.TextView
	CommandView *tview.TextView
	OutputView *tview.TextView
}

// NewUI creates and initializes a new UI.
func NewUI() *UI {
	app := tview.NewApplication()
	app.SetMouseCapture(nil)

	stepsTree := tview.NewTreeView()
	stepsTree.SetRoot(tview.NewTreeNode("Cloud-Init Template Generation").SetColor(tcell.ColorRed))
	stepsTree.SetCurrentNode(stepsTree.GetRoot())
	stepsTree.SetBorder(true).SetTitle("Progress")

	stepView := tview.NewTextView()
	stepView.SetBorder(true)
	stepView.SetTitle("Current Step")
	stepView.SetDynamicColors(true)

	commandView := tview.NewTextView()
	commandView.SetBorder(true)
	commandView.SetTitle("Command")
	commandView.SetDynamicColors(true)
	commandView.SetWrap(true)

	outputView := tview.NewTextView()
	outputView.SetBorder(true)
	outputView.SetTitle("Live Output")
	outputView.SetDynamicColors(true)
	outputView.SetScrollable(true)

	return &UI{
		App:        app,
		StepsTree:  stepsTree,
		StepView:   stepView,
		CommandView: commandView,
		OutputView: outputView,
	}
}

// BuildUITree creates the tree structure in the UI.
func (ui *UI) BuildUITree(images []types.Image, steps []types.Step) []*types.UIImage {
	rootNode := ui.StepsTree.GetRoot()
	uiImages := make([]*types.UIImage, len(images))

	staticSteps := []string{"Download/Verify", "Copy Image"}

	for i, img := range images {
		imgNode := tview.NewTreeNode(fmt.Sprintf("🖼️  %s", img.Name)).SetColor(tcell.ColorGrey)
		rootNode.AddChild(imgNode)

		uiImage := &types.UIImage{Node: imgNode, Image: img}

		for _, stepName := range staticSteps {
			stepNode := tview.NewTreeNode(stepName)
			uiStep := &types.UISTep{Node: stepNode, Name: stepName}
			stepNode.SetReference(uiStep)
			ui.UpdateNodeStatus(stepNode, "pending")
			imgNode.AddChild(stepNode)
			uiImage.Steps = append(uiImage.Steps, uiStep)
		}

		for _, step := range steps {
			stepNode := tview.NewTreeNode(step.Name)
			uiStep := &types.UISTep{Node: stepNode, Name: step.Name}
			stepNode.SetReference(uiStep)
			ui.UpdateNodeStatus(stepNode, "pending")
			imgNode.AddChild(stepNode)
			uiImage.Steps = append(uiImage.Steps, uiStep)
		}
		uiImages[i] = uiImage
	}
	ui.App.QueueUpdateDraw(func() {})
	return uiImages
}

// UpdateNodeStatus updates the icon and color of a node in the tree.
func (ui *UI) UpdateNodeStatus(node *tview.TreeNode, status string) {
	var icon string
	var color tcell.Color
	switch status {
	case "running":
		icon = "⚙️"
		color = tcell.ColorYellow
	case "success":
		icon = "✅"
		color = tcell.ColorGreen
	case "failed":
		icon = "❌"
		color = tcell.ColorRed
	case "skipped":
		icon = "➖"
		color = tcell.ColorDarkGrey
	default: // pending
		icon = "❔"
		color = tcell.ColorGrey
	}
	text := strings.TrimLeft(node.GetText(), "✅⚙️❌➖❔ ")
	ui.App.QueueUpdateDraw(func() {
		node.SetText(fmt.Sprintf("%s %s", icon, text)).SetColor(color)
	})
}

// MarkRemainingStepsAsSkipped marks all pending steps as skipped.
func (ui *UI) MarkRemainingStepsAsSkipped(steps []*types.UISTep) {
	for _, step := range steps {
		text := step.Node.GetText()
		if strings.HasPrefix(text, "❔") {
			ui.UpdateNodeStatus(step.Node, "skipped")
		}
	}
}

// AppendOutput appends text to the output view.
func (ui *UI) AppendOutput(text string) {
	ui.App.QueueUpdateDraw(func() {
		ui.OutputView.Write([]byte(text))
	})
}

// ShowErrorModal displays a modal with an error message.
func (ui *UI) ShowErrorModal(message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Quit"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ui.App.Stop()
		})
	ui.App.SetRoot(modal, false)
}
