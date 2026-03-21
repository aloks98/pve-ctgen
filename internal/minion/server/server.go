package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/aloks98/pve-ctgen/internal/minion/builder"
	minionconfig "github.com/aloks98/pve-ctgen/internal/minion/config"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// Server implements the MinionService gRPC service.
type Server struct {
	pb.UnimplementedMinionServiceServer
	cfg *minionconfig.Config
}

// New creates a new Minion gRPC server.
func New(cfg *minionconfig.Config) *Server {
	return &Server{cfg: cfg}
}

// Build handles a build request with streaming events.
func (s *Server) Build(req *pb.BuildRequest, stream pb.MinionService_BuildServer) error {
	events := make(chan *pb.BuildEvent, 100)

	ctx := stream.Context()
	builderCfg := builder.Config{
		ISOPath:      s.cfg.ISOPath,
		SnippetsPath: s.cfg.SnippetsPath,
		WorkDir:      s.cfg.WorkDir,
	}

	go builder.RunBuild(ctx, req, builderCfg, events)

	for event := range events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}

	return nil
}

// Health returns the node's health information.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	resp := &pb.HealthResponse{
		NodeName: s.cfg.NodeName,
		Version:  "dev",
		Healthy:  true,
	}

	// Get Proxmox version
	out, err := exec.Command("pveversion").Output()
	if err == nil {
		resp.ProxmoxVersion = strings.TrimSpace(string(out))
	}

	// Get available storage
	out, err = exec.Command("pvesm", "status", "--output-format", "json").Output()
	if err == nil {
		// Parse storage names from output
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "\"storage\"") {
				// Simplified parsing; in production parse JSON properly
				resp.AvailableStorage = append(resp.AvailableStorage, line)
			}
		}
	}

	return resp, nil
}

// LaunchVM clones a template and optionally starts the VM.
func (s *Server) LaunchVM(_ context.Context, req *pb.LaunchVMRequest) (*pb.LaunchVMResponse, error) {
	// Clone the template
	args := []string{
		"clone", fmt.Sprintf("%d", req.TemplateId),
		fmt.Sprintf("%d", req.NewVmId),
		"--name", req.Name,
		"--full",
	}

	cmd := exec.Command("qm", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &pb.LaunchVMResponse{
			VmId:    req.NewVmId,
			Success: false,
			Message: fmt.Sprintf("clone failed: %v\n%s", err, string(out)),
		}, nil
	}

	// Apply config overrides
	setArgs := []string{"set", fmt.Sprintf("%d", req.NewVmId)}
	if req.Memory > 0 {
		setArgs = append(setArgs, "--memory", fmt.Sprintf("%d", req.Memory))
	}
	if req.Cores > 0 {
		setArgs = append(setArgs, "--cores", fmt.Sprintf("%d", req.Cores))
	}
	if req.IpConfig != "" {
		setArgs = append(setArgs, "--ipconfig0", req.IpConfig)
	}
	if req.Hostname != "" {
		setArgs = append(setArgs, "--cihostname", req.Hostname)
	}
	if req.Nameserver != "" {
		setArgs = append(setArgs, "--nameserver", req.Nameserver)
	}
	if req.SearchDomain != "" {
		setArgs = append(setArgs, "--searchdomain", req.SearchDomain)
	}
	if req.StartAtBoot {
		setArgs = append(setArgs, "--onboot", "1")
	}

	if len(setArgs) > 2 {
		cmd = exec.Command("qm", setArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("configure failed: %v\n%s", err, string(out)),
			}, nil
		}
	}

	// Start VM if requested
	if req.Start {
		cmd = exec.Command("qm", "start", fmt.Sprintf("%d", req.NewVmId))
		if out, err := cmd.CombinedOutput(); err != nil {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("start failed: %v\n%s", err, string(out)),
			}, nil
		}
	}

	return &pb.LaunchVMResponse{
		VmId:    req.NewVmId,
		Success: true,
		Message: fmt.Sprintf("VM %d created from template %d", req.NewVmId, req.TemplateId),
	}, nil
}

// ListTemplates returns all templates on the node using the Proxmox API via pvesh.
func (s *Server) ListTemplates(_ context.Context, _ *pb.ListTemplatesRequest) (*pb.ListTemplatesResponse, error) {
	hostname, _ := exec.Command("hostname").Output()
	node := strings.TrimSpace(string(hostname))
	if node == "" {
		node = s.cfg.NodeName
	}

	// pvesh returns JSON with all VMs including template flag
	cmd := exec.Command("pvesh", "get", fmt.Sprintf("/nodes/%s/qemu", node), "--output-format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "pvesh failed: %v", err)
	}

	var vms []struct {
		VMID     int    `json:"vmid"`
		Name     string `json:"name"`
		Tags     string `json:"tags"`
		Template int    `json:"template"`
	}

	if err := json.Unmarshal(out, &vms); err != nil {
		return nil, status.Errorf(codes.Internal, "parse pvesh output: %v", err)
	}

	var templates []*pb.TemplateInfo
	for _, vm := range vms {
		if vm.Template != 1 {
			continue
		}
		templates = append(templates, &pb.TemplateInfo{
			VmId: int32(vm.VMID),
			Name: vm.Name,
			Tags: vm.Tags,
		})
	}

	return &pb.ListTemplatesResponse{Templates: templates}, nil
}

// APIKeyInterceptor returns a gRPC unary interceptor that validates the API key.
func APIKeyInterceptor(apiKey string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := validateAPIKey(ctx, apiKey); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// APIKeyStreamInterceptor returns a gRPC stream interceptor that validates the API key.
func APIKeyStreamInterceptor(apiKey string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validateAPIKey(ss.Context(), apiKey); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func validateAPIKey(ctx context.Context, expected string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	keys := md.Get("x-api-key")
	if len(keys) == 0 {
		return status.Error(codes.Unauthenticated, "missing API key")
	}

	if keys[0] != expected {
		return status.Error(codes.Unauthenticated, "invalid API key")
	}

	return nil
}
