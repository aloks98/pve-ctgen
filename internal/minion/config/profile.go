package config

import (
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (c *Config) Profile(name string) (models.BuildProfile, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	p, ok := c.Profiles[name]
	if !ok && name != "default" {
		return p, fmt.Errorf("unknown node profile %q", name)
	}
	if p.Storage == "" {
		p.Storage = "local-lvm"
	}
	if p.SnippetStorage == "" {
		p.SnippetStorage = "local"
	}
	if p.SnippetsPath == "" {
		p.SnippetsPath = c.SnippetsPath
	}
	if p.Bridge == "" {
		p.Bridge = "vmbr0"
	}
	if p.DiskSize == "" {
		p.DiskSize = "32G"
	}
	if p.DiskFormat == "" {
		p.DiskFormat = "raw"
	}
	for _, s := range []string{name, p.Storage, p.SnippetStorage, p.Bridge} {
		if err := validation.Name(s); err != nil {
			return p, err
		}
	}
	if !filepath.IsAbs(p.SnippetsPath) {
		return p, fmt.Errorf("profile snippets_path must be absolute")
	}
	if !regexp.MustCompile(`^[1-9][0-9]*[MGTP]$`).MatchString(p.DiskSize) {
		return p, fmt.Errorf("invalid profile disk_size")
	}
	if _, err := DiskBytes(p.DiskSize); err != nil {
		return p, err
	}
	if p.DiskFormat != "raw" && p.DiskFormat != "qcow2" {
		return p, fmt.Errorf("disk_format must be raw or qcow2")
	}
	if p.AptProxy != "" {
		if err := validation.URL(p.AptProxy); err != nil {
			return p, err
		}
	}
	if p.Timezone != "" {
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			return p, fmt.Errorf("timezone: %w", err)
		}
	}
	for _, key := range p.SSHAuthorizedKeys {
		parts := strings.Fields(key)
		if len(parts) < 2 || strings.ContainsAny(key, "\r\n") || (!strings.HasPrefix(parts[0], "ssh-") && !strings.HasPrefix(parts[0], "ecdsa-")) {
			return p, fmt.Errorf("invalid SSH public key")
		}
	}
	return p, nil
}

func DiskBytes(size string) (uint64, error) {
	if len(size) < 2 {
		return 0, fmt.Errorf("invalid disk size")
	}
	n, err := strconv.ParseUint(size[:len(size)-1], 10, 64)
	if err != nil {
		return 0, err
	}
	units := map[byte]uint64{'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40, 'P': 1 << 50}
	unit := units[size[len(size)-1]]
	if unit == 0 || n == 0 || n > (1<<60)/unit {
		return 0, fmt.Errorf("disk size out of range")
	}
	return n * unit, nil
}
