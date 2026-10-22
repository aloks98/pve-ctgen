package builder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentBuildsUseSeparateWorkspaces(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.Mkdir(bin, 0700)
	os.WriteFile(filepath.Join(bin, "pvesh"), []byte("#!/bin/sh\necho '[]'\n"), 0755)
	os.WriteFile(filepath.Join(bin, "qemu-img"), []byte("#!/bin/sh\necho '{\"format\":\"raw\"}'\n"), 0755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/A", "/B":
			fmt.Fprint(w, r.URL.Path[1:])
		default:
			fmt.Fprintf(w, "%x  A\n%x  B\n", sha256.Sum256([]byte("A")), sha256.Sum256([]byte("B")))
		}
	}))
	defer server.Close()
	cfg := Config{NodeName: "test", ISOPath: filepath.Join(dir, "iso"), SnippetsPath: filepath.Join(dir, "snippets"), WorkDir: filepath.Join(dir, "work"), Profile: models.BuildProfile{}}
	for _, p := range []string{cfg.ISOPath, cfg.SnippetsPath, cfg.WorkDir} {
		os.Mkdir(p, 0700)
	}
	var wg sync.WaitGroup
	for i, name := range []string{"A", "B"} {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			req := &pb.BuildRequest{BuildId: name, Image: &pb.Image{Id: int32(100 + i), Name: "same-name", Url: server.URL + "/" + name, ChecksumUrl: server.URL + "/checksums"}, CloudinitFilename: "same.yaml", CloudinitContent: []byte("#cloud-config\nusers: []\n"), Steps: []*pb.BuildStep{{Name: "assert", Command: "sleep 0.1; test \"$(cat {{.FilePath}})\" = " + name}}}
			events := make(chan *pb.BuildEvent, 100)
			RunBuild(context.Background(), req, cfg, events)
			var last *pb.BuildEvent
			for e := range events {
				last = e
			}
			if last.Type != pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED {
				t.Errorf("%s: %v", name, last)
			}
		}(i, name)
	}
	wg.Wait()
	entries, _ := filepath.Glob(filepath.Join(cfg.WorkDir, "build-*"))
	if len(entries) != 0 {
		t.Fatal("leaked workspaces", entries)
	}
	for _, name := range []string{"A-same.yaml", "B-same.yaml"} {
		if _, err := os.Stat(filepath.Join(cfg.SnippetsPath, name)); err != nil {
			t.Fatal(err)
		}
	}
}
