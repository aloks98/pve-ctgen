package assets_test

import (
	assets "github.com/aloks98/pve-ctgen"
	"github.com/aloks98/pve-ctgen/internal/manager/initresolve"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"os"
	"path/filepath"
	"testing"
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
