package cli

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	minionconfig "github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/minion/server"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Minion gRPC server",
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

			// Create required directories
			for _, dir := range []string{cfg.ISOPath, cfg.SnippetsPath, cfg.WorkDir} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("create directory %s: %w", dir, err)
				}
			}

			lis, err := net.Listen("tcp", cfg.ListenAddress)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}

			grpcServer := grpc.NewServer(
				grpc.UnaryInterceptor(server.APIKeyInterceptor(cfg.APIKey)),
				grpc.StreamInterceptor(server.APIKeyStreamInterceptor(cfg.APIKey)),
			)

			minionServer := server.New(cfg)
			pb.RegisterMinionServiceServer(grpcServer, minionServer)

			// Graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				log.Println("Shutting down...")
				grpcServer.GracefulStop()
			}()

			log.Printf("Minion %q listening on %s\n", cfg.NodeName, cfg.ListenAddress)
			return grpcServer.Serve(lis)
		},
	}
	cmd.Flags().String("config", "/etc/pvectgen/minion.yaml", "Path to minion config file")

	return cmd
}
