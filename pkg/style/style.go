package style

import "github.com/fatih/color"

var (
	// Yellow is a SprintFunc for yellow color.
	Yellow = color.New(color.FgYellow).SprintFunc()
	// Green is a SprintFunc for green color.
	Green  = color.New(color.FgGreen).SprintFunc()
	// Red is a SprintFunc for red color.
	Red    = color.New(color.FgRed).SprintFunc()
)
