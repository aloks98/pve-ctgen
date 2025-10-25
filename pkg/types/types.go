package types

import "github.com/rivo/tview"

// Image represents a cloud image to be processed.
type Image struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ChecksumURL string `json:"checksum_url"`
	Tags        string `json:"tags"`
	Vendor      string `json:"vendor"`
}

// Step represents a command to be executed.
type Step struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// UISTep represents a step in the UI tree.
type UISTep struct {
	Node *tview.TreeNode
	Name string
}

// UIImage represents an image and its UI components.
type UIImage struct {
	Node  *tview.TreeNode
	Image Image
	Steps []*UISTep
}
