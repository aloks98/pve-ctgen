package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectUnknownAndRelativeConfig(t *testing.T) {
	for _, content := range []string{"db_path: /old/database", "data_dir: relative", "data_dir: /tmp/a\n---\ndata_dir: /tmp/b"} {
		p := filepath.Join(t.TempDir(), "config.yaml")
		os.WriteFile(p, []byte(content), 0600)
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted %q", content)
		}
	}
}
