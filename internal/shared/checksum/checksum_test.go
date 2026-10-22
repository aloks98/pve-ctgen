package checksum

import (
	"strings"
	"testing"
)

func TestManifestFormatsAndStrongestHash(t *testing.T) {
	sha256, sha512 := strings.Repeat("a", 64), strings.Repeat("b", 128)
	for _, tc := range []struct{ body, want, algo string }{
		{sha256 + " *disk", sha256, "sha256"},
		{"## disk\nSHA256: " + sha256, sha256, "sha256"},
		{"SHA256 (disk) = " + sha256, sha256, "sha256"},
		{"# MD5 HASH\n" + strings.Repeat("1", 32) + "  disk\n# SHA512 HASH\n" + sha512 + "  disk", sha512, "sha512"},
		{sha256, sha256, "sha256"},
	} {
		hash, algo, err := parseManifest(tc.body, "disk")
		if err != nil || hash != tc.want || algo != tc.algo {
			t.Fatal(hash, algo, err)
		}
	}
	if _, _, err := parseManifest(sha256+" disk\n"+strings.Repeat("c", 64)+" disk", "disk"); err == nil {
		t.Fatal("ambiguous manifest accepted")
	}
	if _, _, err := parseManifest(sha256+" other-disk", "disk"); err == nil {
		t.Fatal("wrong file accepted")
	}
}
