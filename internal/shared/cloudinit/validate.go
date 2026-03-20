package cloudinit

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate checks that content is valid YAML and performs basic cloud-init sanity checks.
// Returns nil if valid, or a descriptive error.
func Validate(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content is empty")
	}

	// Must be valid YAML
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}

	if parsed == nil {
		return fmt.Errorf("YAML parsed to empty document")
	}

	// Must start with #cloud-config if it has the comment (warn if not, but don't block)
	// Cloud-init files conventionally start with #cloud-config but it's in a comment
	// so yaml.Unmarshal won't see it. Check the raw text.
	// We won't enforce this — just validate YAML structure.

	return nil
}

// ValidateStrict performs Validate plus checks for common cloud-init top-level keys.
// Returns a list of warnings (non-fatal) and an error (fatal).
func ValidateStrict(content string) (warnings []string, err error) {
	if err := Validate(content); err != nil {
		return nil, err
	}

	var parsed map[string]interface{}
	yaml.Unmarshal([]byte(content), &parsed)

	knownKeys := map[string]bool{
		"users": true, "packages": true, "runcmd": true, "write_files": true,
		"apt": true, "yum_repos": true, "ssh_authorized_keys": true,
		"timezone": true, "locale": true, "hostname": true, "fqdn": true,
		"manage_etc_hosts": true, "package_update": true, "package_upgrade": true,
		"power_state": true, "bootcmd": true, "mounts": true, "swap": true,
		"disk_setup": true, "fs_setup": true, "growpart": true,
		"ssh_pwauth": true, "chpasswd": true, "password": true,
		"snap": true, "chef": true, "puppet": true, "salt_minion": true,
		"phone_home": true, "final_message": true, "output": true,
		"ntp": true, "rsyslog": true, "ca_certs": true, "ca-certs": true,
		"ansible": true, "lxd": true, "snap_commands": true,
	}

	for key := range parsed {
		if !knownKeys[key] {
			warnings = append(warnings, fmt.Sprintf("unknown cloud-init key: %q", key))
		}
	}

	if !strings.Contains(content, "#cloud-config") {
		warnings = append(warnings, "missing #cloud-config header comment (recommended)")
	}

	return warnings, nil
}
