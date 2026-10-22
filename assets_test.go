package assets_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/aloks98/pve-ctgen"
	"github.com/aloks98/pve-ctgen/internal/manager/initresolve"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/ignition"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"gopkg.in/yaml.v3"
)

func TestBundledDefaultsImportAndResolve(t *testing.T) {
	dir, err := assets.Extract()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Seed(filepath.Join(dir, "config/os_list.json"), filepath.Join(dir, "config/steps.json"), filepath.Join(dir, "cloudinit"), false); err != nil {
		t.Fatal(err)
	}
	templates, err := db.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("empty defaults")
	}
	for _, tmpl := range templates {
		if _, _, err = initresolve.Resolve(db, tmpl); err != nil {
			t.Errorf("%s: %v", tmpl.Name, err)
		}
	}
}

func TestBundledGuestAccessAndOptInServices(t *testing.T) {
	keyFile, err := os.ReadFile("config/guest-ssh-keys.pub")
	if err != nil {
		t.Fatal(err)
	}
	expectedKeys := map[string]bool{}
	for _, line := range strings.Split(string(keyFile), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.HasPrefix(fields[0], "#") {
			expectedKeys[fields[0]+" "+fields[1]] = true
		}
	}
	paths, err := fs.Glob(assets.Files, "cloudinit/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			source, err := assets.Files.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, docker := range []bool{false, true} {
				data, err := cloudinit.ApplyProfile(source, "cloudinit", models.BuildProfile{InstallDocker: docker})
				if err != nil {
					t.Fatal(err)
				}
				var cfg struct {
					DisableRoot bool `yaml:"disable_root"`
					Passwords   bool `yaml:"ssh_pwauth"`
					Upgrade     bool `yaml:"package_upgrade"`
					Users       []struct {
						Name         string
						PrimaryGroup string   `yaml:"primary_group"`
						Locked       bool     `yaml:"lock_passwd"`
						Keys         []string `yaml:"ssh_authorized_keys"`
					}
					Files    []struct{ Path, Content string } `yaml:"write_files"`
					Commands []string                         `yaml:"runcmd"`
				}
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					t.Fatal(err)
				}
				if cfg.DisableRoot || cfg.Passwords || cfg.Upgrade {
					t.Fatal("unexpected default access or upgrade policy")
				}
				for _, name := range []string{"root", "admin"} {
					found := false
					for _, user := range cfg.Users {
						if user.Name == name {
							found = user.Locked && len(user.Keys) == len(expectedKeys)
							for _, key := range user.Keys {
								fields := strings.Fields(key)
								if len(fields) < 2 || !expectedKeys[fields[0]+" "+fields[1]] {
									t.Fatalf("unexpected %s key", name)
								}
							}
							if name == "admin" && (user.PrimaryGroup != "admin") {
								t.Fatal("admin group must be created/reused explicitly")
							}
						}
					}
					if !found {
						t.Fatalf("%s lacks locked-password key access", name)
					}
				}
				motd, access := false, false
				for _, file := range cfg.Files {
					if file.Path == "/etc/profile.d/motd.sh" {
						motd = strings.Contains(file.Content, "Welcome")
					}
					if strings.HasSuffix(file.Path, "00-pvectgen-access.conf") {
						access = strings.Contains(file.Content, "PermitRootLogin prohibit-password") && strings.Contains(file.Content, "PasswordAuthentication no")
					}
					if strings.Contains(file.Path, "cron.weekly") || strings.Contains(file.Path, "maintenance.timer") {
						t.Fatal("maintenance enabled by default")
					}
				}
				if !motd || !access {
					t.Fatal("missing MOTD or root SSH policy")
				}
				installs := 0
				for _, command := range cfg.Commands {
					if command == "/usr/local/sbin/pvectgen-install-docker" {
						installs++
					}
				}
				want := 0
				if docker {
					want = 1
				}
				if installs != want {
					t.Fatalf("Docker scheduled %d times, want %d", installs, want)
				}
			}
		})
	}
}

func TestFlatcarExtensionIsVerifiedBeforeBoot(t *testing.T) {
	source, err := assets.Files.ReadFile("ignition/flatcar-k8s.bu")
	if err != nil {
		t.Fatal(err)
	}
	data, err := ignition.Transpile(source)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Storage struct {
			Files []struct {
				Path     string
				Contents struct {
					Source       string
					Verification struct{ Hash string }
				}
			}
		}
		Systemd struct {
			Units []struct{ Name, Contents string }
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	verified := false
	for _, file := range cfg.Storage.Files {
		if strings.HasSuffix(file.Path, ".raw") {
			verified = strings.HasPrefix(file.Contents.Source, "https://extensions.flatcar.org/") && strings.HasPrefix(file.Contents.Verification.Hash, "sha256-") && len(file.Contents.Verification.Hash) == 71
		}
	}
	if !verified {
		t.Fatal("extension lacks a native verified Ignition download")
	}
	for _, unit := range cfg.Systemd.Units {
		if strings.Contains(unit.Name, "download") || unit.Name == "kubelet.service" && unit.Contents != "" {
			t.Fatal("custom unit overrides packaged extension lifecycle")
		}
	}
}
