package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	managerconfig "github.com/aloks98/pve-ctgen/internal/manager/config"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
)

// NewManagerCmd creates the manager command group.
func NewManagerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Manager mode — manage templates, configs, and trigger builds",
		Long:  "Manager runs on your workstation and manages templates, cloud-init configs, build steps, and triggers builds on Minion nodes via gRPC.",
	}

	cmd.PersistentFlags().StringVar(&cfgPath, "config", "", "Path to manager config (default: ~/.config/pvectgen/config.yaml)")

	cmd.AddCommand(
		newTUICmd(),
		newCloudInitCmd(),
		newTemplateCmd(),
		newStepsCmd(),
		newNodeCmd(),
		newBuildCmd(),
		newBuildsCmd(),
		newVMCmd(),
		newImportCmd(),
	)

	return cmd
}

// openStore loads config and opens the database.
func openStore() (*store.DB, error) {
	cfg, err := managerconfig.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}

// newTabWriter creates a standard tab writer for CLI output.
func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}
