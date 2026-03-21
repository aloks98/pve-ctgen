package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const repo = "aloks98/pve-ctgen"

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckLatest returns the latest release tag from GitHub.
func CheckLatest() (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return "", fmt.Errorf("check latest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return rel.TagName, nil
}

// NeedsUpdate compares current version with latest.
func NeedsUpdate(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	return current != latest && current != "dev"
}

// SelfUpdate downloads the latest release and replaces the current binary.
func SelfUpdate(currentVersion string) error {
	latest, err := CheckLatest()
	if err != nil {
		return err
	}

	if !NeedsUpdate(currentVersion, latest) {
		fmt.Printf("Already up to date (%s)\n", currentVersion)
		return nil
	}

	fmt.Printf("Updating %s -> %s\n", currentVersion, latest)

	// Find the right asset
	assetName := fmt.Sprintf("pvectgen_%s_%s_%s.tar.gz",
		strings.TrimPrefix(latest, "v"),
		runtime.GOOS,
		runtime.GOARCH,
	)

	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var rel Release
	json.NewDecoder(resp.Body).Decode(&rel)

	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (%s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	// Download to temp dir
	tmpDir, err := os.MkdirTemp("", "pvectgen-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, assetName)
	fmt.Printf("Downloading %s...\n", assetName)

	dlResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer dlResp.Body.Close()

	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, dlResp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Extract
	cmd := exec.Command("tar", "-xzf", tarPath, "-C", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract: %w\n%s", err, out)
	}

	newBinary := filepath.Join(tmpDir, "pvectgen")
	if _, err := os.Stat(newBinary); err != nil {
		return fmt.Errorf("binary not found in archive")
	}

	// Replace current binary
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}
	currentBinary, _ = filepath.EvalSymlinks(currentBinary)

	// Try direct replace, fall back to sudo
	if err := replaceBinary(newBinary, currentBinary); err != nil {
		fmt.Println("Direct replace failed, trying with sudo...")
		cmd := exec.Command("sudo", "cp", newBinary, currentBinary)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sudo replace: %w", err)
		}
	}

	fmt.Printf("Updated to %s\n", latest)
	return nil
}

func replaceBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

