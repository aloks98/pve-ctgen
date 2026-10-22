package server

import (
	"context"
	"fmt"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/shared/transport"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Requires the isolated loopback minion and fixture server described in docs/testing.md.
func TestLiveJobs(t *testing.T) {
	path := os.Getenv("PVECTGEN_LIVE_CONFIG")
	if path == "" {
		t.Skip("requires isolated Proxmox test minion")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != "127.0.0.1:50061" || !strings.Contains(cfg.WorkDir, "pvectgen-review-test") {
		t.Fatal("refusing non-test minion")
	}
	pin, err := transport.Fingerprint(cfg.TLSCert)
	if err != nil {
		t.Fatal(err)
	}
	client, err := managergrpc.NewClient(cfg.ListenAddress, cfg.APIKey, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	bad, err := managergrpc.NewClient(cfg.ListenAddress, "wrong-key", pin)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if _, err = bad.Health(context.Background()); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("authentication: %v", err)
	}
	checks, err := client.Preflight(context.Background(), &pb.PreflightRequest{Profile: "broken"})
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, c := range checks.Checks {
		failed = failed || !c.Ok
	}
	if !failed {
		t.Fatal("missing storage passed preflight")
	}
	id := fmt.Sprintf("live-cancel-%d", time.Now().UnixNano())
	marker := filepath.Join(cfg.WorkDir, id+".pid")
	defer os.Remove(marker)
	req := &pb.BuildRequest{BuildId: id, Image: &pb.Image{Id: 998102, Name: "pvectgen-test-job", Url: "http://127.0.0.1:50062/disk.qcow2", ChecksumUrl: "http://127.0.0.1:50062/SHA256SUMS"}, Steps: []*pb.BuildStep{{Name: "wait", Command: fmt.Sprintf("sleep 300 & child=$!; echo \"$child\" > '%s'; echo READY; wait", marker)}}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	streamCtx, disconnect := context.WithCancel(ctx)
	stream, err := client.Build(streamCtx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CancelBuild(context.Background(), id)
	var last int64
	for {
		e, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		last = e.Sequence
		if e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED {
			t.Fatal(e.Message)
		}
		if e.Message == "READY" {
			break
		}
	}
	disconnect() // The job must outlive this RPC.
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		t.Fatalf("disconnect killed build: %v", err)
	}
	resp, err := client.CancelBuild(ctx, id)
	if err != nil || !resp.Cancelled {
		t.Fatalf("cancel: %v %v", resp, err)
	}
	watch, err := client.WatchBuild(ctx, &pb.WatchBuildRequest{BuildId: id, AfterSequence: last})
	if err != nil {
		t.Fatal(err)
	}
	for {
		e, err := watch.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if e.Sequence <= last {
			t.Fatal("replay cursor ignored")
		}
		last = e.Sequence
		if e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED {
			t.Fatal("cancelled job succeeded")
		}
		if e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED {
			break
		}
	}
	data, err = os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil && !strings.Contains(string(data), ") Z ") {
		t.Fatalf("child still running: %s", data)
	}
}
