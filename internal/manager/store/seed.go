package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
)

// Seed imports data from existing JSON and YAML config files into the database.
func (db *DB) Seed(imagesPath, stepsPath, cloudinitDir string) error {
	// Import cloud-init configs
	if cloudinitDir != "" {
		entries, err := os.ReadDir(cloudinitDir)
		if err != nil {
			return fmt.Errorf("read cloudinit dir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(cloudinitDir, entry.Name()))
			if err != nil {
				return fmt.Errorf("read %s: %w", entry.Name(), err)
			}
			if err := cloudinit.Validate(string(content)); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s: %v (importing anyway)\n", entry.Name(), err)
			}
			if _, err := db.CreateCloudInit(entry.Name(), string(content)); err != nil {
				// Skip duplicates
				if !strings.Contains(err.Error(), "UNIQUE constraint") {
					return fmt.Errorf("create cloudinit %s: %w", entry.Name(), err)
				}
			}
		}
	}

	// Import build steps
	if stepsPath != "" {
		steps, err := fileutil.LoadSteps(stepsPath)
		if err != nil {
			return fmt.Errorf("load steps: %w", err)
		}
		if err := db.ResetBuildSteps(steps); err != nil {
			return fmt.Errorf("reset steps: %w", err)
		}
	}

	// Import images as templates
	if imagesPath != "" {
		images, err := fileutil.LoadImages(imagesPath)
		if err != nil {
			return fmt.Errorf("load images: %w", err)
		}
		for _, img := range images {
			// Resolve cloud-init ID
			var ciID *int64
			if img.Vendor != "" {
				ci, err := db.GetCloudInit(img.Vendor)
				if err == nil {
					ciID = &ci.ID
				}
			}
			t := &models.Template{
				VMID:        img.ID,
				Name:        img.Name,
				URL:         img.URL,
				ChecksumURL: img.ChecksumURL,
				Tags:        img.Tags,
				CloudInitID: ciID,
			}
			if _, err := db.CreateTemplate(t); err != nil {
				if !strings.Contains(err.Error(), "UNIQUE constraint") {
					return fmt.Errorf("create template %s: %w", img.Name, err)
				}
			}
		}
	}

	return nil
}
