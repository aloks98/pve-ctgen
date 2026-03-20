package models

import "time"

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

// CloudInitConfig represents a stored cloud-init configuration.
type CloudInitConfig struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Template represents a VM template definition.
type Template struct {
	ID          int64     `json:"id"`
	VMID        int       `json:"vm_id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	ChecksumURL string    `json:"checksum_url"`
	Tags        string    `json:"tags"`
	CloudInitID *int64    `json:"cloudinit_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BuildStep represents a build step definition.
type BuildStep struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Command   string    `json:"command"`
	SortOrder int       `json:"sort_order"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Node represents a Proxmox node (Minion).
type Node struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`         // Proxmox hostname (from minion)
	DisplayName string    `json:"display_name"`  // User-friendly label (set by manager)
	Address     string    `json:"address"`
	APIKey      string    `json:"api_key"`
	CreatedAt   time.Time `json:"created_at"`
}

// Label returns DisplayName if set, otherwise Name.
func (n Node) Label() string {
	if n.DisplayName != "" {
		return n.DisplayName
	}
	return n.Name
}

// Build represents a build execution record.
type Build struct {
	ID          int64      `json:"id"`
	BuildID     string     `json:"build_id"`
	TemplateID  int64      `json:"template_id"`
	NodeID      int64      `json:"node_id"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// BuildStepResult represents the result of a single build step.
type BuildStepResult struct {
	ID          int64      `json:"id"`
	BuildID     string     `json:"build_id"`
	StepName    string     `json:"step_name"`
	StepIndex   int        `json:"step_index"`
	Status      string     `json:"status"`
	Log         string     `json:"log"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
