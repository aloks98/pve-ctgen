package cli

import (
	"context"
	"fmt"
	"time"

	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/shared/token"
	"github.com/spf13/cobra"
)

func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage Proxmox nodes (Minions)",
	}

	cmd.AddCommand(
		newNodeListCmd(),
		newNodeAddCmd(),
		newNodeRemoveCmd(),
		newNodeHealthCmd(),
	)

	return cmd
}

func newNodeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			nodes, err := db.ListNodes()
			if err != nil {
				return err
			}

			if len(nodes) == 0 {
				fmt.Println("No nodes found. Add one with 'pvectgen manager node add --token <token>'.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "NAME\tDISPLAY NAME\tADDRESS\tADDED")
			for _, n := range nodes {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n.Name, n.DisplayName, n.Address, n.CreatedAt.Format("2006-01-02"))
			}
			return w.Flush()
		},
	}
}

func newNodeAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [name] [address]",
		Short: "Add a Minion node (via token or manually)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			tok, _ := cmd.Flags().GetString("token")
			displayName, _ := cmd.Flags().GetString("display-name")

			var name, address, apiKey string
			if tok != "" {
				info, err := token.Decode(tok)
				if err != nil {
					return fmt.Errorf("invalid token: %w", err)
				}
				name = info.Name
				address = info.Address
				apiKey = info.APIKey
			} else if len(args) >= 2 {
				name = args[0]
				address = args[1]
				apiKey, _ = cmd.Flags().GetString("api-key")
				if apiKey == "" {
					return fmt.Errorf("--api-key is required when adding manually")
				}
			} else {
				return fmt.Errorf("provide --token or [name] [address] --api-key")
			}

			// Verify connectivity
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client, err := managergrpc.NewClient(address, apiKey)
			if err != nil {
				return fmt.Errorf("connect failed: %w", err)
			}
			defer client.Close()

			resp, err := client.Health(ctx)
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}

			if _, err := db.CreateNode(name, displayName, address, apiKey); err != nil {
				return err
			}

			label := name
			if displayName != "" {
				label = fmt.Sprintf("%s (%s)", displayName, name)
			}
			fmt.Printf("Verified connection to %s — %s\n", label, resp.ProxmoxVersion)
			fmt.Printf("Added node %s at %s\n", label, address)
			return nil
		},
	}
	cmd.Flags().String("token", "", "Connection token from 'pvectgen minion connect'")
	cmd.Flags().String("display-name", "", "Friendly display name for this node")
	cmd.Flags().String("api-key", "", "API key (for manual add)")
	return cmd
}

func newNodeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := db.DeleteNode(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed node %q\n", args[0])
			return nil
		},
	}
}

func newNodeHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health [name]",
		Short: "Check node health via gRPC",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			if len(args) > 0 {
				node, err := db.GetNode(args[0])
				if err != nil {
					return fmt.Errorf("node %q not found", args[0])
				}
				checkHealth(node.Label(), node.Address, node.APIKey)
			} else {
				nodes, err := db.ListNodes()
				if err != nil {
					return err
				}
				for _, n := range nodes {
					checkHealth(n.Label(), n.Address, n.APIKey)
				}
			}
			return nil
		},
	}
}

func checkHealth(label, address, apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := managergrpc.NewClient(address, apiKey)
	if err != nil {
		fmt.Printf("%-20s UNREACHABLE (%v)\n", label, err)
		return
	}
	defer client.Close()

	resp, err := client.Health(ctx)
	if err != nil {
		fmt.Printf("%-20s ERROR (%v)\n", label, err)
		return
	}

	status := "HEALTHY"
	if !resp.Healthy {
		status = "UNHEALTHY"
	}
	fmt.Printf("%-20s %s  %s  storage: %v\n", label, status, resp.ProxmoxVersion, resp.AvailableStorage)
}
