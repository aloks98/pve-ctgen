package preflight

import (
	"context"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/minion/proxmox"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"strings"
	"testing"
)

func TestInsufficientStorageAndSnippetMismatch(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.NodeName = "test"
	cfg.ISOPath = dir
	cfg.WorkDir = dir
	cfg.JobsDir = dir
	cfg.SnippetsPath = dir
	pve := &proxmox.Client{Node: "test", Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "/storage/local-lvm "):
			return []byte(`{"content":"images"}`), nil
		case strings.Contains(joined, "/storage/local "):
			return []byte(`{"content":"snippets","path":"/different"}`), nil
		case strings.Contains(joined, "/status"):
			return []byte(`{"active":1,"avail":1}`), nil
		default:
			return []byte("available"), nil
		}
	}}
	result := Check(context.Background(), cfg, pve, &pb.PreflightRequest{})
	foundSpace, foundPath := false, false
	for _, c := range result.Checks {
		if c.Ok {
			continue
		}
		foundSpace = foundSpace || strings.Contains(c.Message, "profile disk needs")
		foundPath = foundPath || strings.Contains(c.Message, "snippets_path must match")
	}
	if !foundSpace || !foundPath {
		t.Fatal(fmt.Sprint(result))
	}
}
