package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the Minion configuration.
type Config struct {
	ListenAddress string `yaml:"listen_address"`
	NodeName      string `yaml:"node_name"`
	APIKey        string `yaml:"api_key"`
	ISOPath       string `yaml:"iso_path"`
	SnippetsPath  string `yaml:"snippets_path"`
	WorkDir       string `yaml:"work_dir"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenAddress: "0.0.0.0:50051",
		NodeName:      "",
		APIKey:        "",
		ISOPath:       "/var/lib/vz/template/iso",
		SnippetsPath:  "/var/lib/vz/snippets",
		WorkDir:       "/tmp/pvectgen",
	}
}

// Load reads configuration from a YAML file. Missing fields use defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// Save writes configuration to a YAML file, creating directories as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// EnsureAPIKey generates an API key if one doesn't exist, and saves the config.
func (c *Config) EnsureAPIKey(path string) error {
	if c.APIKey != "" {
		return nil
	}

	key, err := generateAPIKey()
	if err != nil {
		return fmt.Errorf("generate API key: %w", err)
	}
	c.APIKey = key

	return c.Save(path)
}

// EnsureNodeName detects the Proxmox node name if not set in config, and saves.
func (c *Config) EnsureNodeName(path string) error {
	if c.NodeName != "" {
		return nil
	}

	name, err := detectProxmoxNodeName()
	if err != nil {
		return fmt.Errorf("detect node name: %w", err)
	}
	c.NodeName = name

	return c.Save(path)
}

// detectProxmoxNodeName reads the Proxmox node name from the system.
// It tries (in order): /etc/hostname via Proxmox, `hostname`, /etc/hostname file.
func detectProxmoxNodeName() (string, error) {
	// Try pvesh to get the authoritative Proxmox node name
	if out, err := exec.Command("pvesh", "get", "/cluster/status", "--output-format", "json").Output(); err == nil {
		// Parse JSON to find the node entry with type "node" and local=1
		// Simple approach: the hostname command on PVE matches the node name
		_ = out
	}

	// The Proxmox node name is always the system hostname
	if out, err := exec.Command("hostname").Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name, nil
		}
	}

	// Fallback to /etc/hostname
	if data, err := os.ReadFile("/etc/hostname"); err == nil {
		name := strings.TrimSpace(string(data))
		if name != "" {
			return name, nil
		}
	}

	return "", fmt.Errorf("could not detect node name; set node_name in config")
}

// generateAPIKey creates a random API key with the "ak_" prefix.
func generateAPIKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ak_" + hex.EncodeToString(b), nil
}
