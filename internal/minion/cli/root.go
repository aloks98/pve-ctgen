package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

// NewMinionCmd creates the minion command group.
func NewMinionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "minion",
		Short: "Minion mode — headless agent on Proxmox nodes",
		Long:  "Minion runs on each Proxmox node, receives build commands via gRPC, executes them, and streams logs back to the Manager.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := checkProxmoxEnvironment(); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.AddCommand(
		newServeCmd(),
		newConnectCmd(),
	)

	return cmd
}

// checkProxmoxEnvironment verifies that this system is a Proxmox VE node.
func checkProxmoxEnvironment() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("minion must run on a Linux system (detected: %s)", runtime.GOOS)
	}

	// Check for pveversion binary (definitive Proxmox indicator)
	if _, err := exec.LookPath("pveversion"); err != nil {
		return fmt.Errorf("proxmox VE not detected: 'pveversion' command not found.\n" +
			"The minion must run on a Proxmox VE node with qm, pveversion, and pvesm available")
	}

	// Check for qm binary
	if _, err := exec.LookPath("qm"); err != nil {
		return fmt.Errorf("proxmox VE not fully installed: 'qm' command not found")
	}

	// Check that /etc/pve exists (Proxmox cluster filesystem)
	if _, err := os.Stat("/etc/pve"); os.IsNotExist(err) {
		return fmt.Errorf("proxmox VE not detected: /etc/pve does not exist.\n" +
			"The minion must run on a Proxmox VE node")
	}

	return nil
}
