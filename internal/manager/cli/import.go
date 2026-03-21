package cli

import (
	"fmt"

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
	cmd.Flags().String("images", "config/os_list.json", "Path to os_list.json")
	cmd.Flags().String("steps", "config/steps.json", "Path to steps.json")
	cmd.Flags().String("cloudinit-dir", "cloudinit", "Path to cloud-init directory")
	cmd.Flags().Bool("force", false, "Overwrite existing entries")
	return cmd
}
