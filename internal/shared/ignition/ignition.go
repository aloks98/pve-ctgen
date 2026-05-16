// Package ignition handles Butane → Ignition transpilation for Flatcar VMs.
package ignition

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	butane "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
)

// Transpile converts Butane YAML to Ignition JSON.
// Returns the JSON bytes ready to be written as a snippet for Flatcar VMs.
func Transpile(butaneYAML []byte) ([]byte, error) {
	if len(butaneYAML) == 0 {
		return nil, fmt.Errorf("empty butane input")
	}

	out, report, err := butane.TranslateBytes(butaneYAML, common.TranslateBytesOptions{})
	if err != nil {
		return nil, fmt.Errorf("butane translate: %w\n%s", err, report.String())
	}
	if report.IsFatal() {
		return nil, fmt.Errorf("butane fatal errors:\n%s", report.String())
	}

	// Pretty-print for human readability (small overhead, easier debugging)
	var pretty map[string]any
	if err := json.Unmarshal(out, &pretty); err == nil {
		formatted, err := json.MarshalIndent(pretty, "", "  ")
		if err == nil {
			return formatted, nil
		}
	}
	return out, nil
}

// Validate checks that the input parses as valid Butane YAML.
// Returns nil if valid, error with details otherwise.
func Validate(butaneYAML []byte) error {
	_, err := Transpile(butaneYAML)
	return err
}

// IsButane returns true if the filename suggests a Butane source file.
func IsButane(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".bu") || strings.HasSuffix(lower, ".butane")
}

// InjectHostname adds (or replaces) an /etc/hostname file in an Ignition JSON
// config so the booted Flatcar VM gets the given hostname. Returns the
// modified Ignition JSON.
func InjectHostname(ignJSON []byte, hostname string) ([]byte, error) {
	if hostname == "" {
		return ignJSON, nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(ignJSON, &cfg); err != nil {
		return nil, fmt.Errorf("parse ignition json: %w", err)
	}

	storage, _ := cfg["storage"].(map[string]any)
	if storage == nil {
		storage = map[string]any{}
		cfg["storage"] = storage
	}

	var files []any
	if existing, ok := storage["files"].([]any); ok {
		for _, f := range existing {
			if fm, ok := f.(map[string]any); ok {
				if p, _ := fm["path"].(string); p == "/etc/hostname" {
					continue // drop existing, we replace it
				}
			}
			files = append(files, f)
		}
	}

	// Ignition data URL: percent-encode the content. Valid hostnames only
	// contain [A-Za-z0-9.-] so PathEscape is a safe no-op for them.
	source := "data:," + url.PathEscape(hostname) + "%0A"
	files = append(files, map[string]any{
		"path":      "/etc/hostname",
		"mode":      420, // 0644
		"overwrite": true,
		"contents": map[string]any{
			"source": source,
		},
	})
	storage["files"] = files

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal ignition json: %w", err)
	}
	return out, nil
}
