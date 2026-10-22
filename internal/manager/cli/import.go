package cli

import (
	"fmt"
	assets "github.com/aloks98/pve-ctgen"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from existing JSON/YAML config files",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			images, _ := cmd.Flags().GetString("images")
			steps, _ := cmd.Flags().GetString("steps")
			cloudinitDir, _ := cmd.Flags().GetString("cloudinit-dir")
			force, _ := cmd.Flags().GetBool("force")

			dir, err := assets.Extract()
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			if images == "" {
				images = filepath.Join(dir, "config/os_list.json")
			}
			if steps == "" {
				steps = filepath.Join(dir, "config/steps.json")
			}
			if cloudinitDir == "" {
				cloudinitDir = filepath.Join(dir, "cloudinit")
			}
			if err := db.Seed(images, steps, cloudinitDir, force); err != nil {
				return fmt.Errorf("import failed: %w", err)
			}

			if force {
				fmt.Println("Import completed (existing entries overwritten).")
			} else {
				fmt.Println("Import completed (existing entries skipped).")
			}
			return nil
		},
	}
	cmd.Flags().String("images", "", "Path to os_list.json")
	cmd.Flags().String("steps", "", "Path to steps.json")
	cmd.Flags().String("cloudinit-dir", "", "Path to cloud-init directory")
	cmd.Flags().Bool("force", false, "Overwrite existing entries")
	return cmd
}
