package cli

import (
	"fmt"
	"net"

	minionconfig "github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/shared/token"
	"github.com/spf13/cobra"
)

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Show connection token and info for Manager",
		Long:  "Generates (if needed) an API key and displays a connection token that can be used with `pvectgen manager node add --token`.",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := minionconfig.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if err := cfg.EnsureNodeName(configPath); err != nil {
				return fmt.Errorf("detect node name: %w", err)
			}

			if err := cfg.EnsureAPIKey(configPath); err != nil {
				return fmt.Errorf("ensure API key: %w", err)
			}

			// Detect address
			host, port, _ := net.SplitHostPort(cfg.ListenAddress)
			if host == "" || host == "0.0.0.0" {
				host = detectOutboundIP()
			}
			address := net.JoinHostPort(host, port)

			tok, err := token.Encode(token.ConnectionInfo{
				Name:    cfg.NodeName,
				Address: address,
				APIKey:  cfg.APIKey,
			})
			if err != nil {
				return fmt.Errorf("encode token: %w", err)
			}

			fmt.Println()
			fmt.Println("🔗 Minion Connection Info")
			fmt.Printf("  Node:    %s\n", cfg.NodeName)
			fmt.Printf("  Address: %s\n", address)
			fmt.Printf("  API Key: %s...%s\n", cfg.APIKey[:6], cfg.APIKey[len(cfg.APIKey)-4:])
			fmt.Println()
			fmt.Println("Connection token:")
			fmt.Printf("  %s\n", tok)
			fmt.Println()
			fmt.Println("Add this node to your Manager:")
			fmt.Printf("  pvectgen manager node add --token %s\n", tok)
			fmt.Println()

			return nil
		},
	}
	cmd.Flags().String("config", "/etc/pvectgen/minion.yaml", "Path to minion config file")

	return cmd
}

func detectOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
