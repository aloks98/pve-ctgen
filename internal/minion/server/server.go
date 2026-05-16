package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/aloks98/pve-ctgen/internal/minion/builder"
	minionconfig "github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/shared/ignition"
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

// vmExists reports whether a VM/template with the given ID exists on this node.
func vmExists(vmID int32) bool {
	// `qm status <id>` exits non-zero if the VM doesn't exist.
	return exec.Command("qm", "status", fmt.Sprintf("%d", vmID)).Run() == nil
}

// LaunchVM clones a template and optionally starts the VM.
func (s *Server) LaunchVM(_ context.Context, req *pb.LaunchVMRequest) (*pb.LaunchVMResponse, error) {
	vmIDStr := fmt.Sprintf("%d", req.NewVmId)

	// If the target VM ID already exists, either destroy it (overwrite) or fail.
	if vmExists(req.NewVmId) {
		if !req.Overwrite {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("VM %d already exists (enable Overwrite to replace it)", req.NewVmId),
			}, nil
		}
		// Stop (ignore errors — may already be stopped) then purge-destroy.
		_ = exec.Command("qm", "stop", vmIDStr).Run()
		if out, err := exec.Command("qm", "destroy", vmIDStr, "--purge", "--destroy-unreferenced-disks", "1").CombinedOutput(); err != nil {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("overwrite: destroy VM %d failed: %v\n%s", req.NewVmId, err, out),
			}, nil
		}
		log.Printf("VM %d destroyed for overwrite", req.NewVmId)
	}

	// Clone the template
	args := []string{
		"clone", fmt.Sprintf("%d", req.TemplateId),
		vmIDStr,
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

	isIgnition := req.InitType == "ignition"

	// Apply config overrides
	setArgs := []string{"set", fmt.Sprintf("%d", req.NewVmId)}

	// For Ignition (Flatcar): clone inherits cicustom user=...ign. Inject a
	// per-VM /etc/hostname into a copy of that Ignition and repoint cicustom.
	if isIgnition {
		hostname := req.Hostname
		if hostname == "" {
			hostname = req.Name
		}
		if err := s.applyIgnitionHostname(req.NewVmId, hostname, &setArgs); err != nil {
			log.Printf("ignition hostname injection failed: %v", err)
		}
	}

	// For cloud-init VMs, write a per-VM meta-data snippet with hostname.
	if !isIgnition {
		vmHostname := req.Name
		if req.Hostname != "" {
			if parts := strings.SplitN(req.Hostname, ".", 2); len(parts) >= 1 {
				vmHostname = parts[0]
			}
		}
		metaSnippet := fmt.Sprintf("vm-%d-meta.yaml", req.NewVmId)
		metaPath := fmt.Sprintf("%s/%s", s.cfg.SnippetsPath, metaSnippet)
		metaContent := fmt.Sprintf("instance-id: %d\nlocal-hostname: %s\n", req.NewVmId, vmHostname)
		if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
			log.Printf("write meta-data snippet failed: %v", err)
		}

		// Get current cicustom value and append meta snippet
		cicustomOut, _ := exec.Command("bash", "-c",
			fmt.Sprintf("qm config %d | grep cicustom | cut -d' ' -f2-", req.NewVmId)).Output()
		cicustom := strings.TrimSpace(string(cicustomOut))
		if cicustom != "" {
			parts := strings.Split(cicustom, ",")
			var filtered []string
			for _, p := range parts {
				if !strings.HasPrefix(strings.TrimSpace(p), "meta=") {
					filtered = append(filtered, strings.TrimSpace(p))
				}
			}
			cicustom = strings.Join(filtered, ",")
			cicustom += fmt.Sprintf(",meta=local:snippets/%s", metaSnippet)
		} else {
			cicustom = fmt.Sprintf("meta=local:snippets/%s", metaSnippet)
		}
		setArgs = append(setArgs, "--cicustom", cicustom)
	}
	if req.Memory > 0 {
		setArgs = append(setArgs, "--memory", fmt.Sprintf("%d", req.Memory))
	}
	if req.Cores > 0 {
		setArgs = append(setArgs, "--cores", fmt.Sprintf("%d", req.Cores))
	}
	if req.IpConfig != "" {
		setArgs = append(setArgs, "--ipconfig0", req.IpConfig)
	}
	// If hostname is a FQDN, extract searchdomain from it
	if req.Hostname != "" {
		if parts := strings.SplitN(req.Hostname, ".", 2); len(parts) == 2 && req.SearchDomain == "" {
			setArgs = append(setArgs, "--searchdomain", parts[1])
		}
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
		log.Printf("LaunchVM configure: qm %s", strings.Join(setArgs, " "))
		if out, err := cmd.CombinedOutput(); err != nil {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("configure failed: %v\ncommand: qm %s\n%s", err, strings.Join(setArgs, " "), string(out)),
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

// applyIgnitionHostname reads the cloned VM's inherited Ignition snippet,
// injects /etc/hostname, writes a per-VM snippet, and appends the new
// cicustom user= override to setArgs.
func (s *Server) applyIgnitionHostname(vmID int32, hostname string, setArgs *[]string) error {
	// Find the base Ignition snippet the clone inherited.
	out, err := exec.Command("bash", "-c",
		fmt.Sprintf("qm config %d | grep cicustom | cut -d' ' -f2-", vmID)).Output()
	if err != nil {
		return fmt.Errorf("read cicustom: %w", err)
	}
	cicustom := strings.TrimSpace(string(out))
	if cicustom == "" {
		return fmt.Errorf("no cicustom on VM %d", vmID)
	}

	// Extract the user= snippet filename
	var baseFile string
	for _, part := range strings.Split(cicustom, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "user=") {
			// user=local:snippets/foo.ign  ->  foo.ign
			val := strings.TrimPrefix(part, "user=")
			if idx := strings.LastIndex(val, "/"); idx >= 0 {
				baseFile = val[idx+1:]
			} else {
				baseFile = val
			}
		}
	}
	if baseFile == "" {
		return fmt.Errorf("no user= snippet in cicustom %q", cicustom)
	}

	basePath := fmt.Sprintf("%s/%s", s.cfg.SnippetsPath, baseFile)
	baseJSON, err := os.ReadFile(basePath)
	if err != nil {
		return fmt.Errorf("read base ignition %s: %w", basePath, err)
	}

	modified, err := ignition.InjectHostname(baseJSON, hostname)
	if err != nil {
		return fmt.Errorf("inject hostname: %w", err)
	}

	perVMFile := fmt.Sprintf("vm-%d.ign", vmID)
	perVMPath := fmt.Sprintf("%s/%s", s.cfg.SnippetsPath, perVMFile)
	if err := os.WriteFile(perVMPath, modified, 0644); err != nil {
		return fmt.Errorf("write per-vm ignition: %w", err)
	}

	*setArgs = append(*setArgs, "--cicustom",
		fmt.Sprintf("user=local:snippets/%s", perVMFile))
	log.Printf("VM %d: ignition hostname %q -> %s", vmID, hostname, perVMFile)
	return nil
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
