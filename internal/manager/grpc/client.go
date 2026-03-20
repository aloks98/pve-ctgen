package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
func NewClient(address, apiKey string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
