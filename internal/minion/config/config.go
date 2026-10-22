package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/filelock"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/internal/shared/configutil"
	"github.com/aloks98/pve-ctgen/internal/shared/transport"
	"gopkg.in/yaml.v3"
)

// Config holds the Minion configuration.
type Config struct {
	DefaultProfile string                         `yaml:"default_profile"`
	Profiles       map[string]models.BuildProfile `yaml:"profiles"`
	JobsDir        string                         `yaml:"jobs_dir"`
	ListenAddress  string                         `yaml:"listen_address"`
	NodeName       string                         `yaml:"node_name"`
	APIKey         string                         `yaml:"api_key"`
	TLSCert        string                         `yaml:"tls_cert"`
	TLSKey         string                         `yaml:"tls_key"`
	ISOPath        string                         `yaml:"iso_path"`
	SnippetsPath   string                         `yaml:"snippets_path"`
	WorkDir        string                         `yaml:"work_dir"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ListenAddress: "0.0.0.0:50051",
		JobsDir:       "/var/lib/pvectgen/jobs",
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

	if err := configutil.Decode(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if _, _, err := net.SplitHostPort(cfg.ListenAddress); err != nil {
		return nil, fmt.Errorf("listen_address: %w", err)
	}
	for _, p := range []string{cfg.ISOPath, cfg.SnippetsPath, cfg.WorkDir, cfg.JobsDir} {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("minion directories must be absolute paths")
		}
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return nil, fmt.Errorf("tls_cert and tls_key must be set together")
	}
	for _, p := range []string{cfg.TLSCert, cfg.TLSKey} {
		if p != "" && !filepath.IsAbs(p) {
			return nil, fmt.Errorf("TLS paths must be absolute")
		}
	}
	for name := range cfg.Profiles {
		if _, err := cfg.Profile(name); err != nil {
			return nil, err
		}
	}
	if _, err := cfg.Profile(""); err != nil {
		return nil, err
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

	return fileutil.AtomicWrite(path, data, 0600)
}

// EnsureAPIKey generates an API key if one doesn't exist, and saves the config.
func (c *Config) EnsureAPIKey(path string) error {
	if c.APIKey != "" {
		return os.Chmod(path, 0600)
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

// EnsureTLS provisions an identity once; custom certificate paths must already exist.
func (c *Config) EnsureTLS(path string) error {
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("tls_cert and tls_key must be set together")
	}
	if c.TLSCert != "" {
		return nil
	}
	dir := filepath.Dir(path)
	c.TLSCert = filepath.Join(dir, "tls.crt")
	c.TLSKey = filepath.Join(dir, "tls.key")
	if _, err := os.Stat(c.TLSCert); os.IsNotExist(err) {
		if _, err := os.Stat(c.TLSKey); !os.IsNotExist(err) {
			return fmt.Errorf("TLS key exists without certificate; restore the certificate before starting")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := transport.Generate(c.TLSCert, c.TLSKey, c.NodeName); err != nil {
			return err
		}
	}
	return c.Save(path)
}

// Prepare serializes identity provisioning across connect and serve processes.
func Prepare(path string) (*Config, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	release, err := filelock.Acquire(ctx, path+".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if err = cfg.EnsureNodeName(path); err != nil {
		return nil, err
	}
	if err = cfg.EnsureAPIKey(path); err != nil {
		return nil, err
	}
	if err = cfg.EnsureTLS(path); err != nil {
		return nil, err
	}
	return cfg, nil
}
