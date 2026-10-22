package cli

import (
	"github.com/aloks98/pve-ctgen/internal/manager/tui"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	var macKeys bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			return tui.Run(db, Version, macKeys)
		},
	}
	cmd.Flags().BoolVar(&macKeys, "mac-keys", false, "Show Command+G hints (requires a Mac terminal binding; see docs/tui.md)")
	return cmd
}
