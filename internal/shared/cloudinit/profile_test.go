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

func TestDockerOptIn(t *testing.T) {
	source := []byte("#cloud-config\nwrite_files:\n  - path: /usr/local/sbin/pvectgen-install-docker\n    permissions: '0755'\n    content: '#!/bin/sh'\nruncmd:\n  - [echo, ready]\n")
	for _, enabled := range []bool{false, true} {
		profile := models.BuildProfile{InstallDocker: enabled}
		out, err := ApplyProfile(source, "cloudinit", profile)
		if err != nil {
			t.Fatal(err)
		}
		// Applying a profile twice must not schedule duplicate installations.
		out, err = ApplyProfile(out, "cloudinit", profile)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Commands []interface{} `yaml:"runcmd"`
		}
		if err := yaml.Unmarshal(out, &cfg); err != nil {
			t.Fatal(err)
		}
		want := 1
		if enabled {
			want = 2
		}
		if len(cfg.Commands) != want {
			t.Fatalf("unexpected commands: %s", out)
		}
	}
	for _, test := range []struct{ source, kind string }{
		{"{}", "cloudinit"}, {"{}", "ignition"},
	} {
		if _, err := ApplyProfile([]byte(test.source), test.kind, models.BuildProfile{InstallDocker: true}); err == nil {
			t.Fatalf("accepted unsupported Docker setup: %s", test.kind)
		}
	}
}

func TestMaintenancePolicy(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name             string
		profile          models.BuildProfile
		upgrade          bool
		files            int
		cleanup, journal bool
	}{
		{name: "preserve", upgrade: true},
		{name: "disable upgrade", profile: models.BuildProfile{PackageUpgrade: &no}},
		{name: "enable upgrade", profile: models.BuildProfile{PackageUpgrade: &yes}, upgrade: true},
		{name: "cleanup only", profile: models.BuildProfile{PackageCleanup: true}, upgrade: true, files: 3, cleanup: true},
		{name: "journal only", profile: models.BuildProfile{JournalRetentionDays: 30}, upgrade: true, files: 3, journal: true},
		{name: "both", profile: models.BuildProfile{PackageCleanup: true, JournalRetentionDays: 30}, upgrade: true, files: 3, cleanup: true, journal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ApplyProfile([]byte("package_upgrade: true\n"), "cloudinit", tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			out, err = ApplyProfile(out, "cloudinit", tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				Upgrade  bool                       `yaml:"package_upgrade"`
				Files    []struct{ Content string } `yaml:"write_files"`
				Commands []string                   `yaml:"runcmd"`
			}
			if err := yaml.Unmarshal(out, &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Upgrade != tc.upgrade || len(cfg.Files) != tc.files {
				t.Fatalf("unexpected policy: %s", out)
			}
			if tc.files > 0 {
				if len(cfg.Commands) != 2 || strings.Contains(cfg.Files[0].Content, "autoremove") != tc.cleanup || strings.Contains(cfg.Files[0].Content, "--vacuum-time=30d") != tc.journal {
					t.Fatalf("incorrect maintenance: %s", out)
				}
			} else if len(cfg.Commands) != 0 {
				t.Fatalf("default scheduled maintenance: %s", out)
			}
		})
	}
	for _, p := range []models.BuildProfile{{JournalRetentionDays: -1}, {JournalRetentionDays: 36501}, {PackageCleanup: true}, {PackageUpgrade: &no}} {
		if _, err := ApplyProfile([]byte(`{}`), "ignition", p); err == nil {
			t.Fatalf("unsupported Ignition policy accepted: %+v", p)
		}
	}
}

func TestEmptyProfileDocumentsAndIgnitionRevocation(t *testing.T) {
	for _, kind := range []string{"ignition", "cloudinit"} {
		if _, err := ApplyProfile([]byte("null"), kind, models.BuildProfile{PackageCleanup: kind == "cloudinit"}); err == nil {
			t.Fatalf("accepted null %s", kind)
		}
	}
	out, err := ApplyProfile([]byte(`{"ignition":{"version":"3.3.0"}}`), "ignition", models.BuildProfile{SSHKeysMode: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "sshAuthorizedKeys") {
		t.Fatalf("empty replacement emitted invalid null keys: %s", out)
	}
}
