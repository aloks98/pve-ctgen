package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the Manager configuration.
type Config struct {
	DataDir     string `yaml:"data_dir"`
	DefaultNode string `yaml:"default_node"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		DataDir:     filepath.Join(home, ".config", "pvectgen"),
		DefaultNode: "",
	}
}

// Load reads configuration from a YAML file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "pvectgen", "config.yaml")
	}

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

	// Expand ~
	if len(cfg.DataDir) > 0 && cfg.DataDir[0] == '~' {
		home, _ := os.UserHomeDir()
		cfg.DataDir = filepath.Join(home, cfg.DataDir[1:])
	}

	return cfg, nil
}
