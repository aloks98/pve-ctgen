// Package proxmox owns discovery and guarded mutations of Proxmox resources.
package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Runner func(context.Context, string, ...string) ([]byte, error)
type Client struct {
	Run  Runner
	Node string
}
type VM struct {
	ID       int32  `json:"vmid"`
	Node     string `json:"node"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Template int    `json:"template"`
	Status   string `json:"status"`
}

func New(node string) *Client {
	return &Client{Node: node, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}
}
func ValidID(id int32) bool { return id >= 100 && id <= 999999999 }
func (c *Client) Inventory(ctx context.Context) (map[int32]VM, error) {
	out, err := c.Run(ctx, "pvesh", "get", "/cluster/resources", "--type", "vm", "--output-format", "json")
	if err != nil {
		return nil, fmt.Errorf("read cluster inventory: %w: %s", err, out)
	}
	var list []VM
	if err = json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse cluster inventory: %w", err)
	}
	result := make(map[int32]VM, len(list))
	for _, v := range list {
		result[v.ID] = v
	}
	return result, nil
}
func (c *Client) CheckClone(ctx context.Context, source, target int32, overwrite bool) (bool, error) {
	if !ValidID(source) || !ValidID(target) {
		return false, fmt.Errorf("VM IDs must be between 100 and 999999999")
	}
	if source == target {
		return false, fmt.Errorf("source template and destination VM IDs must differ")
	}
	vms, err := c.Inventory(ctx)
	if err != nil {
		return false, err
	}
	src, ok := vms[source]
	if !ok || src.Type != "qemu" {
		return false, fmt.Errorf("source %d is not an existing QEMU template", source)
	}
	if c.Node != "" && src.Node != c.Node {
		return false, fmt.Errorf("source %d belongs to node %s", source, src.Node)
	}
	sourceConfig, err := c.Config(ctx, source)
	if err != nil {
		return false, err
	}
	if sourceConfig["template"] != "1" {
		return false, fmt.Errorf("source %d is not a template", source)
	}

	dst, exists := vms[target]
	if !exists {
		return false, nil
	}
	if !overwrite {
		return false, fmt.Errorf("VM %d already exists (enable overwrite to replace it)", target)
	}
	if dst.Type != "qemu" || dst.Node != src.Node {
		return false, fmt.Errorf("refusing to overwrite template, container, or remote VM %d", target)
	}
	targetConfig, err := c.Config(ctx, target)
	if err != nil {
		return false, err
	}
	if targetConfig["template"] == "1" {
		return false, fmt.Errorf("refusing to overwrite template %d", target)
	}
	return true, nil
}
func (c *Client) CheckBuild(ctx context.Context, id int32, overwrite bool) (bool, error) {
	if !ValidID(id) {
		return false, fmt.Errorf("VM ID must be between 100 and 999999999")
	}
	vms, err := c.Inventory(ctx)
	if err != nil {
		return false, err
	}
	vm, exists := vms[id]
	if !exists {
		return false, nil
	}
	if !overwrite {
		return false, fmt.Errorf("VM %d already exists; rebuilding requires explicit overwrite", id)
	}
	if vm.Type != "qemu" || (c.Node != "" && vm.Node != c.Node) {
		return false, fmt.Errorf("refusing to replace non-template or remote VM %d", id)
	}
	cfg, err := c.Config(ctx, id)
	if err != nil {
		return false, err
	}
	if cfg["template"] != "1" {
		return false, fmt.Errorf("refusing to replace non-template VM %d", id)
	}

	return true, nil
}
func (c *Client) Destroy(ctx context.Context, id int32, stop bool) error {
	idStr := strconv.Itoa(int(id))
	if stop {
		if out, err := c.Run(ctx, "qm", "stop", idStr); err != nil {
			return fmt.Errorf("stop VM %d: %w: %s", id, err, out)
		}
	}
	if out, err := c.Run(ctx, "qm", "destroy", idStr, "--purge", "--destroy-unreferenced-disks", "1"); err != nil {
		return fmt.Errorf("destroy VM %d: %w: %s", id, err, out)
	}
	return nil
}

// Config reads authoritative configuration; cluster status can lag behind new VMs.
func (c *Client) Config(ctx context.Context, id int32) (map[string]string, error) {
	out, err := c.Run(ctx, "qm", "config", strconv.Itoa(int(id)))
	if err != nil {
		return nil, fmt.Errorf("read VM %d config: %w: %s", id, err, out)
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			result[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return result, nil
}
