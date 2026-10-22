package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/minion/jobs"
	"github.com/aloks98/pve-ctgen/internal/minion/preflight"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	"google.golang.org/protobuf/proto"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/aloks98/pve-ctgen/internal/minion/builder"
	minionconfig "github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/minion/proxmox"
	"github.com/aloks98/pve-ctgen/internal/shared/filelock"
	"github.com/aloks98/pve-ctgen/internal/shared/ignition"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// Server implements the MinionService gRPC service.
type Server struct {
	pb.UnimplementedMinionServiceServer
	cfg     *minionconfig.Config
	pve     *proxmox.Client
	jobOnce sync.Once
	jobs    *jobs.Manager
	jobErr  error
}

// New creates a new Minion gRPC server.
func New(cfg *minionconfig.Config) *Server {
	return &Server{cfg: cfg, pve: proxmox.New(cfg.NodeName)}
}

func (s *Server) InitJobs() error {
	s.jobOnce.Do(func() {
		cfg := builder.Config{ISOPath: s.cfg.ISOPath, SnippetsPath: s.cfg.SnippetsPath, WorkDir: s.cfg.WorkDir, NodeName: s.cfg.NodeName}
		s.jobs, s.jobErr = jobs.New(s.cfg.JobsDir, func(ctx context.Context, req *pb.BuildRequest, events chan<- *pb.BuildEvent) {
			p, _ := s.cfg.Profile(req.Options["profile"])
			buildCfg := cfg
			buildCfg.Profile = p
			builder.RunBuild(ctx, req, buildCfg, events)
		})
	})
	return s.jobErr
}
func (s *Server) Close() {
	if s.jobs != nil {
		s.jobs.Close()
	}
}
func (s *Server) Build(req *pb.BuildRequest, stream pb.MinionService_BuildServer) error {
	if err := validation.Build(req); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.InitJobs(); err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 60*time.Second)
	defer cancel()
	checks := preflight.Check(ctx, s.cfg, s.pve, &pb.PreflightRequest{Profile: req.Options["profile"], Build: req})
	if err := preflight.Error(checks); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	req = proto.Clone(req).(*pb.BuildRequest)
	profile, _ := s.cfg.Profile(req.Options["profile"])
	content, err := cloudinit.ApplyProfile(req.CloudinitContent, req.InitType, profile)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	req.CloudinitContent = content
	if err := s.jobs.Start(req); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return s.jobs.Watch(stream.Context(), req.BuildId, 0, stream.Send)
}
func (s *Server) WatchBuild(req *pb.WatchBuildRequest, stream pb.MinionService_WatchBuildServer) error {
	if err := s.InitJobs(); err != nil {
		return err
	}
	return s.jobs.Watch(stream.Context(), req.BuildId, req.AfterSequence, stream.Send)
}
func (s *Server) CancelBuild(ctx context.Context, req *pb.CancelBuildRequest) (*pb.CancelBuildResponse, error) {
	if err := s.InitJobs(); err != nil {
		return nil, err
	}
	ok, err := s.jobs.Cancel(req.BuildId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &pb.CancelBuildResponse{Cancelled: ok, Message: "Cancellation requested; watch the build for its final status"}, nil
}

// Health returns the node's health information.
func (s *Server) Preflight(ctx context.Context, req *pb.PreflightRequest) (*pb.PreflightResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return preflight.Check(ctx, s.cfg, s.pve, req), nil
}
func (s *Server) Health(ctx context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	resp := &pb.HealthResponse{NodeName: s.cfg.NodeName, Version: "dev", Healthy: true}
	out, err := s.pve.Run(ctx, "pveversion")
	if err != nil {
		resp.Healthy = false
	} else {
		resp.ProxmoxVersion = strings.TrimSpace(string(out))
	}
	out, err = s.pve.Run(ctx, "pvesh", "get", "/nodes/"+s.cfg.NodeName+"/storage", "--output-format", "json")
	var storages []struct {
		Storage string `json:"storage"`
		Active  int    `json:"active"`
	}
	if err == nil {
		err = json.Unmarshal(out, &storages)
	}
	if err != nil {
		resp.Healthy = false
	} else {
		for _, storage := range storages {
			if storage.Active == 1 {
				resp.AvailableStorage = append(resp.AvailableStorage, storage.Storage)
			}
		}
	}
	return resp, nil
}

// LaunchVM validates the source and destination before any mutation.
func (s *Server) LaunchVM(ctx context.Context, req *pb.LaunchVMRequest) (*pb.LaunchVMResponse, error) {
	if err := validation.Launch(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	ids := []int{int(req.TemplateId), int(req.NewVmId)}
	sort.Ints(ids)
	if ids[0] == ids[1] || !proxmox.ValidID(req.TemplateId) || !proxmox.ValidID(req.NewVmId) {
		return nil, status.Error(codes.InvalidArgument, "source and destination must be distinct valid VM IDs")
	}
	for _, id := range ids {
		release, err := filelock.Acquire(ctx, filepath.Join(s.cfg.WorkDir, "locks", fmt.Sprintf("vm-%d.lock", id)))
		if err != nil {
			return nil, err
		}
		defer release()
	}
	replace, err := s.pve.CheckClone(ctx, req.TemplateId, req.NewVmId, req.Overwrite)
	if err != nil {
		return &pb.LaunchVMResponse{VmId: req.NewVmId, Message: err.Error()}, nil
	}
	profile, err := s.cfg.Profile(req.Profile)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := preflight.Error(preflight.Check(ctx, s.cfg, s.pve, &pb.PreflightRequest{Profile: req.Profile})); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	sourceConfig, err := s.pve.Config(ctx, req.TemplateId)
	if err != nil {
		return nil, err
	}
	// Use the source template's init type as the authoritative value.
	if strings.Contains(sourceConfig["tags"], "ignition") {
		req = proto.Clone(req).(*pb.LaunchVMRequest)
		req.InitType = "ignition"
	}
	if req.InitType == "ignition" && req.IpConfig != "" && req.IpConfig != "ip=dhcp" {
		return nil, status.Error(codes.InvalidArgument, "static networking for Ignition must be configured in the Butane file")
	}
	if replace {
		if err := s.pve.Destroy(ctx, req.NewVmId, true); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
	}
	vmIDStr := fmt.Sprintf("%d", req.NewVmId)

	// Clone the template
	args := []string{
		"clone", fmt.Sprintf("%d", req.TemplateId),
		vmIDStr,
		"--name", req.Name,
		"--full",
	}

	out, err := s.pve.Run(ctx, "qm", args...)
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
		if err := s.applyIgnitionHostname(ctx, req.NewVmId, hostname, profile.SnippetStorage, profile.SnippetsPath, &setArgs); err != nil {
			return nil, status.Errorf(codes.Internal, "prepare ignition: %v", err)
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
		metaPath := filepath.Join(profile.SnippetsPath, metaSnippet)
		metaContent := fmt.Sprintf("instance-id: %d\nlocal-hostname: %s\n", req.NewVmId, vmHostname)
		if err := fileutil.AtomicWrite(metaPath, []byte(metaContent), 0600); err != nil {
			return nil, status.Errorf(codes.Internal, "prepare metadata: %v", err)
		}

		// Get current cicustom value and append meta snippet
		current, err := s.pve.Config(ctx, req.NewVmId)
		if err != nil {
			return nil, err
		}
		cicustom := current["cicustom"]
		if cicustom != "" {
			parts := strings.Split(cicustom, ",")
			var filtered []string
			for _, p := range parts {
				if !strings.HasPrefix(strings.TrimSpace(p), "meta=") {
					filtered = append(filtered, strings.TrimSpace(p))
				}
			}
			cicustom = strings.Join(filtered, ",")
			cicustom += fmt.Sprintf(",meta=%s:snippets/%s", profile.SnippetStorage, metaSnippet)
		} else {
			cicustom = fmt.Sprintf("meta=%s:snippets/%s", profile.SnippetStorage, metaSnippet)
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

		log.Printf("LaunchVM configure: qm %s", strings.Join(setArgs, " "))
		if out, err := s.pve.Run(ctx, "qm", setArgs...); err != nil {
			return &pb.LaunchVMResponse{
				VmId:    req.NewVmId,
				Success: false,
				Message: fmt.Sprintf("configure failed: %v\ncommand: qm %s\n%s", err, strings.Join(setArgs, " "), string(out)),
			}, nil
		}
	}

	// Start VM if requested
	if req.Start {
		if out, err := s.pve.Run(ctx, "qm", "start", vmIDStr); err != nil {
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
func (s *Server) applyIgnitionHostname(ctx context.Context, vmID int32, hostname, snippetStorage, snippetsPath string, setArgs *[]string) error {
	current, err := s.pve.Config(ctx, vmID)
	if err != nil {
		return err
	}
	cicustom := current["cicustom"]
	if cicustom == "" {
		return fmt.Errorf("no cicustom on VM %d", vmID)
	}

	// Extract the user= snippet filename
	var baseFile string
	for _, part := range strings.Split(cicustom, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "user=") {
			baseFile = strings.TrimPrefix(part, "user=")
		}
	}
	if baseFile == "" {
		return fmt.Errorf("no user= snippet in cicustom %q", cicustom)
	}

	out, err := s.pve.Run(ctx, "pvesm", "path", baseFile)
	if err != nil {
		return err
	}
	basePath := strings.TrimSpace(string(out))
	baseJSON, err := os.ReadFile(basePath)
	if err != nil {
		return fmt.Errorf("read base ignition %s: %w", basePath, err)
	}

	modified, err := ignition.InjectHostname(baseJSON, hostname)
	if err != nil {
		return fmt.Errorf("inject hostname: %w", err)
	}

	perVMFile := fmt.Sprintf("vm-%d.ign", vmID)
	perVMPath := filepath.Join(snippetsPath, perVMFile)
	if err := fileutil.AtomicWrite(perVMPath, modified, 0600); err != nil {
		return fmt.Errorf("write per-vm ignition: %w", err)
	}

	*setArgs = append(*setArgs, "--cicustom",
		fmt.Sprintf("user=%s:snippets/%s", snippetStorage, perVMFile))
	log.Printf("VM %d: ignition hostname %q -> %s", vmID, hostname, perVMFile)
	return nil
}

// ListTemplates returns all templates on the node using the Proxmox API via pvesh.
func (s *Server) ListTemplates(ctx context.Context, _ *pb.ListTemplatesRequest) (*pb.ListTemplatesResponse, error) {
	return s.listVMs(ctx, true)
}
func (s *Server) ListVMs(ctx context.Context, _ *pb.ListTemplatesRequest) (*pb.ListTemplatesResponse, error) {
	return s.listVMs(ctx, false)
}
func (s *Server) listVMs(ctx context.Context, templatesOnly bool) (*pb.ListTemplatesResponse, error) {
	out, err := s.pve.Run(ctx, "pvesh", "get", fmt.Sprintf("/nodes/%s/qemu", s.cfg.NodeName), "--output-format", "json")
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
		if templatesOnly && vm.Template != 1 {
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

	if expected == "" || subtle.ConstantTimeCompare([]byte(keys[0]), []byte(expected)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid API key")
	}

	return nil
}
