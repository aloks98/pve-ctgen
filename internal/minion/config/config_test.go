package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentProvisioningKeepsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minion.yaml")
	os.WriteFile(path, []byte("node_name: test-node\n"), 0600)
	var wg sync.WaitGroup
	keys := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := Prepare(path)
			if err != nil {
				t.Error(err)
				return
			}
			keys <- cfg.APIKey
		}()
	}
	wg.Wait()
	close(keys)
	var first string
	for key := range keys {
		if first == "" {
			first = key
		}
		if key != first {
			t.Fatal("identity changed")
		}
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, cfg.TLSKey} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0600 {
			t.Fatal("credential permissions", st.Mode())
		}
	}
}
func TestProfilesRejectInvalidSettings(t *testing.T) {
	for _, input := range []string{"profiles:\n  default:\n    disk_format: other\n", "default_profile: missing\n", "unknown: true\n", "tls_cert: cert.pem\ntls_key: key.pem\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		os.WriteFile(path, []byte(input), 0600)
		if _, err := Load(path); err == nil {
			t.Fatal("accepted", input)
		}
	}
}
