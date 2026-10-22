package cloudinit

import (
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"strings"
	"testing"
)

func TestProfileKeysAndHostname(t *testing.T) {
	data, err := ApplyProfile([]byte("#cloud-config\nusers:\n  - name: admin\n    lock_passwd: true\n"), "cloudinit", models.BuildProfile{SSHAuthorizedKeys: []string{"ssh-ed25519 test"}, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ssh-ed25519 test") || strings.Contains(string(data), "hostname:") {
		t.Fatal(string(data))
	}
}
