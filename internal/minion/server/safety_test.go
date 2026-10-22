package server

import (
	"context"
	"github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/minion/proxmox"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCloneSourceNeverDestroyed(t *testing.T) {
	s := New(&config.Config{NodeName: "test"})
	calls := 0
	s.pve.Run = func(context.Context, string, ...string) ([]byte, error) { calls++; return nil, nil }
	resp, err := s.LaunchVM(context.Background(), &pb.LaunchVMRequest{TemplateId: 100, NewVmId: 100, Overwrite: true, Name: "pvectgen-test"})
	if (err == nil && resp.Success) || calls != 0 {
		t.Fatalf("unsafe request reached runner: resp=%v err=%v calls=%d", resp, err, calls)
	}
}

// TestLiveGuards runs only against an explicitly selected Proxmox node. It
// creates diskless test fixtures only after verifying their IDs are unused.
func TestLiveGuards(t *testing.T) {
	node := os.Getenv("PVECTGEN_LIVE_TEST_NODE")
	if node == "" {
		t.Skip("set PVECTGEN_LIVE_TEST_NODE on a Proxmox host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p := proxmox.New(node)
	inv, err := p.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int32{998100, 998101, 998102} {
		if _, exists := inv[id]; exists {
			t.Fatalf("test ID %d already in use", id)
		}
	}
	for _, id := range []string{"998100", "998101"} {
		args := []string{"create", id, "--name", "pvectgen-test-" + id, "--memory", "128"}
		if id == "998100" {
			args = append(args, "--template", "1")
		}
		if out, err := p.Run(ctx, "qm", args...); err != nil {
			t.Fatalf("fixture: %v %s", err, out)
		}
		t.Cleanup(func() {
			out, err := p.Run(context.Background(), "qm", "config", id)
			if err != nil || !strings.Contains(string(out), "name: pvectgen-test-"+id) {
				t.Errorf("refusing cleanup of unrecognized fixture %s", id)
				return
			}
			if out, err = p.Run(context.Background(), "qm", "destroy", id); err != nil {
				t.Errorf("cleanup: %v %s", err, out)
			}
		})
	}
	s := New(&config.Config{NodeName: node, WorkDir: t.TempDir()})
	for _, r := range []*pb.LaunchVMRequest{
		{TemplateId: 998100, NewVmId: 998100, Overwrite: true, Name: "pvectgen-test"},
		{TemplateId: 998102, NewVmId: 998101, Overwrite: true, Name: "pvectgen-test"},
		{TemplateId: 998100, NewVmId: 998101, Name: "pvectgen-test"},
	} {
		resp, err := s.LaunchVM(ctx, r)
		if err == nil && resp.Success {
			t.Fatalf("unsafe launch succeeded: %v", r)
		}
	}
	for _, id := range []int32{998100, 998101} {
		cfg, err := p.Config(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(cfg["name"], "pvectgen-test-") {
			t.Fatalf("fixture %d changed", id)
		}
		if id == 998100 && cfg["template"] != "1" {
			t.Fatal("source is no longer a template")
		}
	}

	t.Log("same-ID overwrite, missing-source overwrite, and overwrite opt-in guards passed on Proxmox")
}
