package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/aloks98/pve-ctgen/internal/shared/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// Client wraps a gRPC connection to a Minion.
type Client struct {
	conn   *grpc.ClientConn
	client pb.MinionServiceClient
	apiKey string
}

// NewClient creates a new gRPC client connected to the given address.
func NewClient(address, apiKey string, fingerprints ...string) (*Client, error) {
	var pin string
	if len(fingerprints) > 0 {
		pin = fingerprints[0]
	}
	tlsConfig, err := transport.ClientTLS(pin)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewMinionServiceClient(conn),
		apiKey: apiKey,
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) authCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-api-key", c.apiKey)
}

// Health calls the Health RPC.
func (c *Client) Health(ctx context.Context) (*pb.HealthResponse, error) {
	return c.client.Health(c.authCtx(ctx), &pb.HealthRequest{})
}

// Build calls the Build RPC and returns a stream of events.
func (c *Client) Build(ctx context.Context, req *pb.BuildRequest) (pb.MinionService_BuildClient, error) {
	return c.client.Build(c.authCtx(ctx), req)
}

// LaunchVM calls the LaunchVM RPC.
func (c *Client) LaunchVM(ctx context.Context, req *pb.LaunchVMRequest) (*pb.LaunchVMResponse, error) {
	return c.client.LaunchVM(c.authCtx(ctx), req)
}

// ListTemplates calls the ListTemplates RPC.
func (c *Client) ListTemplates(ctx context.Context) (*pb.ListTemplatesResponse, error) {
	return c.client.ListTemplates(c.authCtx(ctx), &pb.ListTemplatesRequest{})
}

func (c *Client) WatchBuild(ctx context.Context, req *pb.WatchBuildRequest) (pb.MinionService_WatchBuildClient, error) {
	return c.client.WatchBuild(c.authCtx(ctx), req)
}
func (c *Client) CancelBuild(ctx context.Context, id string) (*pb.CancelBuildResponse, error) {
	return c.client.CancelBuild(c.authCtx(ctx), &pb.CancelBuildRequest{BuildId: id})
}
func (c *Client) Preflight(ctx context.Context, req *pb.PreflightRequest) (*pb.PreflightResponse, error) {
	return c.client.Preflight(c.authCtx(ctx), req)
}
func (c *Client) ListVMs(ctx context.Context) (*pb.ListTemplatesResponse, error) {
	return c.client.ListVMs(c.authCtx(ctx), &pb.ListTemplatesRequest{})
}
