package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	managercli "github.com/aloks98/pve-ctgen/internal/manager/cli"
	minioncli "github.com/aloks98/pve-ctgen/internal/minion/cli"
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

	rootCmd.AddCommand(managercli.NewManagerCmd())
	rootCmd.AddCommand(minioncli.NewMinionCmd())
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("pvectgen %s\n", version)
		},
	})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
