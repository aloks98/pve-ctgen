package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	managercli "github.com/aloks98/pve-ctgen/internal/manager/cli"
	minioncli "github.com/aloks98/pve-ctgen/internal/minion/cli"
	"github.com/aloks98/pve-ctgen/internal/update"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "pvectgen",
		Short: "Proxmox VE Cloud-Init Template Generator",
		Long:  "A manager-minion system for creating Proxmox VE virtual machine templates from cloud images.",
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	managercli.Version = version
	rootCmd.AddCommand(managercli.NewManagerCmd())
	rootCmd.AddCommand(minioncli.NewMinionCmd())
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("pvectgen %s\n", version)
			latest, err := update.CheckLatest()
			if err == nil && update.NeedsUpdate(version, latest) {
				fmt.Printf("Update available: %s -> %s\n", version, latest)
				fmt.Println("Run 'pvectgen update' to update.")
			}
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update pvectgen to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return update.SelfUpdate(version)
		},
	})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
