package cloudinit

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// Maintenance is opt-in. A systemd timer works on minimal cloud images that do
// not ship cron; package cleanup and journal retention remain independent.
func applyMaintenance(cfg map[string]interface{}, p models.BuildProfile) {
	if p.PackageUpgrade != nil {
		cfg["package_upgrade"] = *p.PackageUpgrade
	}
	if !p.PackageCleanup && p.JournalRetentionDays == 0 {
		return
	}
	script := "#!/bin/sh\nset -eu\n"
	if p.PackageCleanup {
		script += `if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get autoremove -y -qq
  apt-get autoclean -y -qq
elif command -v dnf >/dev/null 2>&1; then
  dnf autoremove -y -q
else
  echo "No supported package manager for cleanup" >&2
  exit 1
fi
`
	}
	if p.JournalRetentionDays > 0 {
		script += fmt.Sprintf("journalctl --rotate --vacuum-time=%dd\n", p.JournalRetentionDays)
	}
	files := []map[string]interface{}{
		{"path": "/usr/local/sbin/pvectgen-maintenance", "permissions": "0755", "content": script},
		{"path": "/etc/systemd/system/pvectgen-maintenance.service", "permissions": "0644", "content": `[Unit]
Description=Opt-in guest maintenance
After=cloud-final.service
[Service]
Type=oneshot
ExecStart=/usr/local/sbin/pvectgen-maintenance
`},
		{"path": "/etc/systemd/system/pvectgen-maintenance.timer", "permissions": "0644", "content": `[Unit]
Description=Weekly opt-in guest maintenance
[Timer]
OnCalendar=weekly
RandomizedDelaySec=1h
Persistent=true
[Install]
WantedBy=timers.target
`},
	}
	existing, _ := cfg["write_files"].([]interface{})
	for _, file := range files {
		found := false
		for i, entry := range existing {
			if f, ok := entry.(map[string]interface{}); ok && f["path"] == file["path"] {
				existing[i] = file
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, file)
		}
	}
	cfg["write_files"] = existing
	appendCommand(cfg, "systemctl daemon-reload")
	appendCommand(cfg, "systemctl enable --now pvectgen-maintenance.timer")
}

func appendCommand(cfg map[string]interface{}, command string) {
	commands, _ := cfg["runcmd"].([]interface{})
	for _, existing := range commands {
		if existing == command {
			return
		}
	}
	cfg["runcmd"] = append(commands, command)
}
