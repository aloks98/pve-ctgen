package models

import "time"

// Image represents a cloud image to be processed (from os_list.json seed data).
type Image struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ChecksumURL string `json:"checksum_url"`
	Tags        string `json:"tags"`
	Vendor      string `json:"vendor"`
}

// Step represents a command to be executed (from steps.json seed data).
type Step struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// CloudInitConfig represents a stored cloud-init configuration.
type CloudInitConfig struct {
	Name      string    `yaml:"-" json:"name"`           // derived from filename
	Content   string    `yaml:"-" json:"content"`        // file contents
	CreatedAt time.Time `yaml:"-" json:"created_at"`
	UpdatedAt time.Time `yaml:"-" json:"updated_at"`
}

// Template represents a VM template definition.
type Template struct {
	VMID        int    `yaml:"vm_id" json:"vm_id"`
	Name        string `yaml:"name" json:"name"`
	URL         string `yaml:"url" json:"url"`
	ChecksumURL string `yaml:"checksum_url,omitempty" json:"checksum_url"`
	Tags        string `yaml:"tags,omitempty" json:"tags"`
	CloudInit   string `yaml:"cloudinit,omitempty" json:"cloudinit"` // cloud-init filename
}

// BuildStep represents a build step definition.
type BuildStep struct {
	Name    string `yaml:"name" json:"name"`
	Command string `yaml:"command" json:"command"`
}

// Node represents a Proxmox node (Minion).
type Node struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name,omitempty" json:"display_name"`
	Address     string `yaml:"address" json:"address"`
	APIKey      string `yaml:"api_key" json:"api_key"`
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
	BuildID     string          `yaml:"build_id" json:"build_id"`
	Template    string          `yaml:"template" json:"template"`
	Node        string          `yaml:"node" json:"node"`
	Status      string          `yaml:"status" json:"status"`
	StartedAt   *time.Time      `yaml:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt *time.Time      `yaml:"completed_at,omitempty" json:"completed_at,omitempty"`
	Steps       []BuildStepResult `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// BuildStepResult represents the result of a single build step.
type BuildStepResult struct {
	StepName    string     `yaml:"step_name" json:"step_name"`
	StepIndex   int        `yaml:"step_index" json:"step_index"`
	Status      string     `yaml:"status" json:"status"`
	Log         string     `yaml:"log,omitempty" json:"log,omitempty"`
	StartedAt   *time.Time `yaml:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty" json:"completed_at,omitempty"`
}
