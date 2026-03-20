package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// ProgressFunc is a callback for reporting download progress.
// downloaded is the number of bytes downloaded so far, total is the total size (-1 if unknown).
type ProgressFunc func(downloaded, total int64)

// File downloads a file from the given URL to the specified path.
// The progressFn callback is called periodically with download progress.
// If ctx is cancelled, the download is aborted.
func File(ctx context.Context, url, filePath string, progressFn ProgressFunc) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer file.Close()

	totalSize := resp.ContentLength

	if progressFn == nil {
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		return nil
	}

	pw := &progressWriter{
		writer:     file,
		total:      totalSize,
		progressFn: progressFn,
	}

	_, err = io.Copy(pw, resp.Body)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	return nil
}

type progressWriter struct {
	writer     io.Writer
	total      int64
	downloaded int64
	progressFn ProgressFunc
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.downloaded += int64(n)
	pw.progressFn(pw.downloaded, pw.total)
	return n, err
}
