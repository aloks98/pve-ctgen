package update

import (
	"archive/tar"
	"compress/gzip"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"v1.9.0", "v1.10.0", true}, {"v2.0.0", "v1.9.0", false}, {"1.2.3", "v1.2.3", false}, {"dev", "v1.0.0", false}} {
		if NeedsUpdate(tc.a, tc.b) != tc.want {
			t.Fatal(tc)
		}
	}
}
func TestExtractRejectsLinksAndAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "pvectgen", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	gz.Close()
	f.Close()
	if _, err := extractBinary(path); err == nil {
		t.Fatal("symlink accepted")
	}
	dst := filepath.Join(dir, "binary")
	os.WriteFile(dst, []byte("old"), 0755)
	old, _ := os.Open(dst)
	defer old.Close()
	if err := fileutil.AtomicWrite(dst, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	old.Read(buf)
	if string(buf) != "old" {
		t.Fatal("old inode modified")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "new" {
		t.Fatal(string(data))
	}
}
