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

			if err := db.Seed(images, steps, cloudinitDir); err != nil {
				return fmt.Errorf("import failed: %w", err)
			}

			fmt.Println("Import completed successfully!")
			return nil
		},
	}
	cmd.Flags().String("images", "config/os_list.json", "Path to os_list.json")
	cmd.Flags().String("steps", "config/steps.json", "Path to steps.json")
	cmd.Flags().String("cloudinit-dir", "cloudinit", "Path to cloud-init directory")
	return cmd
}
