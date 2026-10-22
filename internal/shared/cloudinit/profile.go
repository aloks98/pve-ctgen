package cloudinit

import (
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"gopkg.in/yaml.v3"
	"strings"
)

// ApplyProfile adds explicit node/site settings without embedding them in defaults.
func ApplyProfile(content []byte, initType string, p models.BuildProfile) ([]byte, error) {
	if p.SSHKeysMode != "" && p.SSHKeysMode != "merge" && p.SSHKeysMode != "replace" {
		return nil, fmt.Errorf("ssh_keys_mode must be merge or replace")
	}
	if p.JournalRetentionDays < 0 || p.JournalRetentionDays > 36500 {
		return nil, fmt.Errorf("journal_retention_days must be between 0 and 36500")
	}
	if len(content) == 0 {
		if p.InstallDocker || p.PackageUpgrade != nil || p.PackageCleanup || p.JournalRetentionDays != 0 {
			return nil, fmt.Errorf("guest provisioning options require cloud-init content")
		}
		return content, nil
	}
	var cfg map[string]interface{}
	if initType == "ignition" {
		if p.InstallDocker || p.PackageUpgrade != nil || p.PackageCleanup || p.JournalRetentionDays != 0 {
			return nil, fmt.Errorf("Docker and package/journal maintenance options require cloud-init; Flatcar manages its own OS and containerd")
		}
		if err := json.Unmarshal(content, &cfg); err != nil {
			return nil, err
		}
		if len(p.SSHAuthorizedKeys) > 0 || p.SSHKeysMode == "replace" {
			passwd, _ := cfg["passwd"].(map[string]interface{})
			if passwd == nil {
				passwd = map[string]interface{}{}
				cfg["passwd"] = passwd
			}
			users, _ := passwd["users"].([]interface{})
			found := false
			for _, u := range users {
				if user, ok := u.(map[string]interface{}); ok && (user["name"] == "core" || user["name"] == "root") {
					setUserKeys(user, "sshAuthorizedKeys", p)
					found = found || user["name"] == "core"
				}
			}
			if !found {
				users = append(users, map[string]interface{}{"name": "core", "sshAuthorizedKeys": p.SSHAuthorizedKeys})
			}
			passwd["users"] = users
		}
		return json.Marshal(cfg)
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	if len(p.SSHAuthorizedKeys) > 0 || p.SSHKeysMode == "replace" {
		users, _ := cfg["users"].([]interface{})
		found := false
		for _, u := range users {
			if user, ok := u.(map[string]interface{}); ok && (user["name"] == "admin" || (user["name"] == "root" && cfg["disable_root"] == false)) {
				setUserKeys(user, "ssh_authorized_keys", p)
				found = found || user["name"] == "admin"
			}
		}
		if !found {
			return nil, fmt.Errorf("site SSH keys require an admin user in cloud-init")
		}
	}
	if p.Timezone != "" {
		cfg["timezone"] = p.Timezone
	}
	if p.AptProxy != "" {
		apt, _ := cfg["apt"].(map[string]interface{})
		if apt == nil {
			apt = map[string]interface{}{}
		}
		apt["proxy"] = p.AptProxy
		cfg["apt"] = apt
	}
	if p.InstallDocker {
		const installer = "/usr/local/sbin/pvectgen-install-docker"
		files, _ := cfg["write_files"].([]interface{})
		found := false
		for _, file := range files {
			if f, ok := file.(map[string]interface{}); ok && f["path"] == installer {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("install_docker requires %s in cloud-init write_files", installer)
		}
		appendCommand(cfg, installer)
	}
	applyMaintenance(cfg, p)
	data, err := yaml.Marshal(cfg)
	return append([]byte("#cloud-config\n"), data...), err
}

// Preserve existing access unless replacement is explicitly requested. Comments
// do not identify keys: deduplicate by key type and public key payload.
func setUserKeys(user map[string]interface{}, field string, p models.BuildProfile) {
	var keys []string
	if p.SSHKeysMode != "replace" {
		if existing, ok := user[field].([]interface{}); ok {
			for _, key := range existing {
				if value, ok := key.(string); ok {
					keys = append(keys, value)
				}
			}
		}
	}
	keys = append(keys, p.SSHAuthorizedKeys...)
	unique := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		parts := strings.Fields(key)
		identity := key
		if len(parts) >= 2 {
			identity = parts[0] + " " + parts[1]
		}
		if !seen[identity] {
			unique = append(unique, key)
			seen[identity] = true
		}
	}
	if len(unique) == 0 {
		delete(user, field)
	} else {
		user[field] = unique
	}
}
