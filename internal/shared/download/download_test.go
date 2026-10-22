package download

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifiedAtomicCache(t *testing.T) {
	content := "valid image"
	bad := false
	requests := 0
	checksumStatus := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sum" {
			w.WriteHeader(checksumStatus)
			fmt.Fprintf(w, "%x  image", sha256.Sum256([]byte(content)))
			return
		}
		requests++
		if bad {
			fmt.Fprint(w, "corrupt")
		} else {
			fmt.Fprint(w, content)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "cache")
	run := func() error { return Verified(context.Background(), srv.URL+"/image", srv.URL+"/sum", path, nil) }
	bad = true
	if err := run(); err == nil {
		t.Fatal("mismatch accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("bad image published")
	}
	bad = false
	if err := run(); err != nil {
		t.Fatal(err)
	}
	before := requests
	if err := run(); err != nil || requests != before {
		t.Fatal("valid cache not reused")
	}
	content = "new expected image"
	bad = true
	if err := run(); err == nil {
		t.Fatal("mismatch accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "valid image" {
		t.Fatal("existing cache damaged")
	}
	checksumStatus = 404
	if err := run(); err == nil {
		t.Fatal("HTTP error accepted")
	}
	if err := Verified(context.Background(), srv.URL, "", path, nil); err == nil {
		t.Fatal("missing checksum accepted")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Fatal("partial file retained")
		}
	}
}
func TestCancelledDownloadPreservesFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "image")
	os.WriteFile(path, []byte("old"), 0600)
	if err := File(ctx, "http://127.0.0.1:1", path, nil); err == nil {
		t.Fatal("cancel ignored")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "old" {
		t.Fatal("file modified")
	}
}
