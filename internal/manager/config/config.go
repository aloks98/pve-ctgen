package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the Manager configuration.
type Config struct {
	DBPath      string `yaml:"db_path"`
	DefaultNode string `yaml:"default_node"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		DBPath:      filepath.Join(home, ".config", "pvectgen", "pvectgen.db"),
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
	if len(cfg.DBPath) > 0 && cfg.DBPath[0] == '~' {
		home, _ := os.UserHomeDir()
		cfg.DBPath = filepath.Join(home, cfg.DBPath[1:])
	}

	return cfg, nil
}
