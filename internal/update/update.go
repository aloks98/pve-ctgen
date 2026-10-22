package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/download"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "aloks98/pve-ctgen"

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

var versionRE = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func release(ctx context.Context, version string) (*Release, error) {
	endpoint := "latest"
	if version != "" {
		if !versionRE.MatchString(version) {
			return nil, fmt.Errorf("version must be vMAJOR.MINOR.PATCH")
		}
		endpoint = "tags/v" + strings.TrimPrefix(version, "v")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repo+"/releases/"+endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}
	var rel Release
	err = json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&rel)
	if err != nil {
		return nil, err
	}
	if !versionRE.MatchString(rel.TagName) {
		return nil, fmt.Errorf("invalid release version %q", rel.TagName)
	}
	return &rel, nil
}
func CheckLatest() (string, error) {
	r, e := release(context.Background(), "")
	if e != nil {
		return "", e
	}
	return r.TagName, nil
}
func NeedsUpdate(current, latest string) bool {
	a, b := versionRE.FindStringSubmatch(current), versionRE.FindStringSubmatch(latest)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.ParseUint(a[i], 10, 64)
		y, _ := strconv.ParseUint(b[i], 10, 64)
		if x != y {
			return y > x
		}
	}
	return false
}
func SelfUpdate(current string, versions ...string) error {
	target := ""
	if len(versions) > 0 {
		target = versions[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	rel, err := release(ctx, target)
	if err != nil {
		return err
	}
	if target == "" && !NeedsUpdate(current, rel.TagName) {
		fmt.Printf("No newer stable release for %s\n", current)
		return nil
	}
	name := fmt.Sprintf("pvectgen_%s_%s_%s.tar.gz", strings.TrimPrefix(rel.TagName, "v"), runtime.GOOS, runtime.GOARCH)
	var archiveURL, checksumURL string
	for _, a := range rel.Assets {
		if a.Name == name {
			archiveURL = a.BrowserDownloadURL
		}
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
		}
	}
	if archiveURL == "" || checksumURL == "" {
		return fmt.Errorf("release missing %s or checksums.txt", name)
	}
	dir, err := os.MkdirTemp("", "pvectgen-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, name)
	if err = download.Verified(ctx, archiveURL, checksumURL, archive, nil); err != nil {
		return err
	}
	binary, err := extractBinary(archive)
	if err != nil {
		return err
	}
	dst, err := os.Executable()
	if err != nil {
		return err
	}
	dst, err = filepath.EvalSymlinks(dst)
	if err != nil {
		return err
	}
	if err = fileutil.AtomicWrite(dst, binary, 0755); err != nil {
		return fmt.Errorf("replace executable atomically (run update with write access to %s): %w", filepath.Dir(dst), err)
	}
	fmt.Printf("Updated to %s; restart any running minion service to use it\n", rel.TagName)
	return nil
}
func extractBinary(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("binary not found in archive")
		}
		if err != nil {
			return nil, err
		}
		if h.Name != "pvectgen" && h.Name != "./pvectgen" {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > 256*1024*1024 {
			return nil, fmt.Errorf("invalid binary archive entry")
		}
		return io.ReadAll(io.LimitReader(tr, 256*1024*1024+1))
	}
}
