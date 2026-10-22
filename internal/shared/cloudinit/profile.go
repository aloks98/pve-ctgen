package cloudinit

import (
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"gopkg.in/yaml.v3"
)

// ApplyProfile adds explicit node/site settings without embedding them in defaults.
func ApplyProfile(content []byte, initType string, p models.BuildProfile) ([]byte, error) {
	if len(content) == 0 {
		return content, nil
	}
	var cfg map[string]interface{}
	if initType == "ignition" {
		if err := json.Unmarshal(content, &cfg); err != nil {
			return nil, err
		}
		if len(p.SSHAuthorizedKeys) > 0 {
			passwd, _ := cfg["passwd"].(map[string]interface{})
			if passwd == nil {
				passwd = map[string]interface{}{}
				cfg["passwd"] = passwd
			}
			users, _ := passwd["users"].([]interface{})
			found := false
			for _, u := range users {
				if user, ok := u.(map[string]interface{}); ok && user["name"] == "core" {
					user["sshAuthorizedKeys"] = p.SSHAuthorizedKeys
					found = true
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
	if len(p.SSHAuthorizedKeys) > 0 {
		users, _ := cfg["users"].([]interface{})
		found := false
		for _, u := range users {
			if user, ok := u.(map[string]interface{}); ok && user["name"] == "admin" {
				user["ssh_authorized_keys"] = p.SSHAuthorizedKeys
				found = true
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
	data, err := yaml.Marshal(cfg)
	return append([]byte("#cloud-config\n"), data...), err
}
