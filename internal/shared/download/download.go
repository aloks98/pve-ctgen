package download

import (
	"context"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/checksum"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type ProgressFunc func(downloaded, total int64)

// File publishes a complete download atomically. Existing files survive failures.
func File(ctx context.Context, url, path string, progress ProgressFunc) error {
	return fetch(ctx, url, path, progress, nil)
}

// Verified reuses a matching cache entry or verifies a fresh download before publishing it.
func Verified(ctx context.Context, imageURL, checksumURL, path string, progress ProgressFunc) error {
	if checksumURL == "" {
		return fmt.Errorf("checksum URL is required")
	}
	u, err := url.Parse(imageURL)
	if err != nil {
		return err
	}
	expected, algo, err := checksum.GetExpectedChecksumContext(ctx, checksumURL, filepath.Base(u.Path))
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	if algo != "sha256" && algo != "sha512" {
		return fmt.Errorf("image checksum must use SHA-256 or SHA-512")
	}
	verify := func(p string) error {
		actual, err := checksum.CalculateFileChecksum(p, algo)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("image checksum mismatch (%s)", algo)
		}
		return nil
	}
	if err := verify(path); err == nil {
		return nil
	}
	return fetch(ctx, imageURL, path, progress, verify)
}
func fetch(ctx context.Context, url, path string, progress ProgressFunc, verify func(string) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP status: %s", resp.Status)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	var w io.Writer = f
	if progress != nil {
		w = &progressWriter{writer: f, total: resp.ContentLength, progressFn: progress}
	}
	if _, err = io.Copy(w, resp.Body); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if verify != nil {
		if err = verify(f.Name()); err != nil {
			return err
		}
	}
	return os.Rename(f.Name(), path)
}

type progressWriter struct {
	writer            io.Writer
	total, downloaded int64
	progressFn        ProgressFunc
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.writer.Write(b)
	p.downloaded += int64(n)
	p.progressFn(p.downloaded, p.total)
	return n, err
}
