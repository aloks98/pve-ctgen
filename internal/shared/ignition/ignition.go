// Package ignition handles Butane → Ignition transpilation for Flatcar VMs.
package ignition

import (
	"encoding/json"
	"fmt"
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
