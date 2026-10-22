// Package assets contains the defaults shipped with every binary.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed config/*.json cloudinit/*.yaml ignition/*.bu
var Files embed.FS

// Extract materializes defaults for the file-based importer. The caller removes dir.
func Extract() (string, error) {
	dir, err := os.MkdirTemp("", "pvectgen-defaults-")
	if err != nil {
		return "", err
	}
	err = fs.WalkDir(Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		data, err := Files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0600)
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}
