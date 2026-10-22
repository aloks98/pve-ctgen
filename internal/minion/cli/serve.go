package cli

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

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

			cfg, err := minionconfig.Prepare(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			pair, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
			if err != nil {
				return fmt.Errorf("load TLS identity: %w", err)
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
				grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13})),
				grpc.UnaryInterceptor(server.APIKeyInterceptor(cfg.APIKey)),
				grpc.StreamInterceptor(server.APIKeyStreamInterceptor(cfg.APIKey)),
			)

			minionServer := server.New(cfg)
			if err := minionServer.InitJobs(); err != nil {
				return err
			}
			defer minionServer.Close()
			pb.RegisterMinionServiceServer(grpcServer, minionServer)

			// Graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				log.Println("Shutting down...")
				done := make(chan struct{})
				go func() { minionServer.Close(); grpcServer.GracefulStop(); close(done) }()
				select {
				case <-done:
				case <-time.After(15 * time.Second):
					grpcServer.Stop()
				}
			}()

			log.Printf("Minion %q listening on %s\n", cfg.NodeName, cfg.ListenAddress)
			return grpcServer.Serve(lis)
		},
	}
	cmd.Flags().String("config", "/etc/pvectgen/minion.yaml", "Path to minion config file")

	return cmd
}
