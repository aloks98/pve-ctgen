package cloudinit

import (
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"gopkg.in/yaml.v3"
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

func TestProfileMergesRootKeysOnlyWhenEnabled(t *testing.T) {
	for _, disabled := range []string{"false", "true"} {
		source := "#cloud-config\ndisable_root: " + disabled + "\nusers:\n  - name: root\n    ssh_authorized_keys: [ssh-ed25519 old root]\n  - name: admin\n    ssh_authorized_keys: [ssh-ed25519 old admin]\n"
		out, err := ApplyProfile([]byte(source), "cloudinit", models.BuildProfile{SSHAuthorizedKeys: []string{"ssh-ed25519 old duplicate-comment", "ssh-ed25519 new laptop"}})
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Users []struct {
				Name string
				Keys []string `yaml:"ssh_authorized_keys"`
			}
		}
		if err := yaml.Unmarshal(out, &cfg); err != nil {
			t.Fatal(err)
		}
		wantRoot := 1
		if disabled == "false" {
			wantRoot = 2
		}
		if len(cfg.Users[0].Keys) != wantRoot || len(cfg.Users[1].Keys) != 2 {
			t.Fatalf("unexpected access: %s", out)
		}
		out, err = ApplyProfile([]byte(source), "cloudinit", models.BuildProfile{SSHKeysMode: "replace", SSHAuthorizedKeys: []string{"ssh-ed25519 new laptop"}})
		if err != nil {
			t.Fatal(err)
		}
		yaml.Unmarshal(out, &cfg)
		if len(cfg.Users[1].Keys) != 1 || cfg.Users[1].Keys[0] != "ssh-ed25519 new laptop" {
			t.Fatalf("replacement failed: %s", out)
		}
	}
}

func TestIgnitionKeysMergeAndExplicitRevocation(t *testing.T) {
	source := []byte(`{"passwd":{"users":[{"name":"core","sshAuthorizedKeys":["ssh-ed25519 old core"]},{"name":"root","sshAuthorizedKeys":["ssh-ed25519 old root"]}]}}`)
	out, err := ApplyProfile(source, "ignition", models.BuildProfile{SSHAuthorizedKeys: []string{"ssh-ed25519 new laptop"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "new laptop") != 2 || strings.Count(string(out), "old ") != 2 {
		t.Fatalf("lost access: %s", out)
	}
	out, err = ApplyProfile(source, "ignition", models.BuildProfile{SSHKeysMode: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "sshAuthorizedKeys") {
		t.Fatalf("revocation failed: %s", out)
	}
}
