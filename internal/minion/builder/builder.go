package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aloks98/pve-ctgen/internal/minion/executor"
	"github.com/aloks98/pve-ctgen/internal/shared/checksum"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/download"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// Config holds builder runtime paths.
type Config struct {
	ISOPath      string
	SnippetsPath string
	WorkDir      string
}

// RunBuild orchestrates a full build: download, checksum, cloud-init, execute steps.
// Events are sent on the events channel. The channel is closed when the build completes.
func RunBuild(ctx context.Context, req *pb.BuildRequest, cfg Config, events chan<- *pb.BuildEvent) {
	defer close(events)

	buildID := req.BuildId
	img := req.Image
	steps := req.Steps

	sendEvent := func(e *pb.BuildEvent) {
		e.BuildId = buildID
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}

	// Validate and write cloud-init config
	if len(req.CloudinitContent) > 0 && req.CloudinitFilename != "" {
		if err := cloudinit.Validate(string(req.CloudinitContent)); err != nil {
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: fmt.Sprintf("invalid cloud-init YAML: %v", err),
			})
			return
		}
		ciPath := filepath.Join(cfg.SnippetsPath, req.CloudinitFilename)
		if err := os.WriteFile(ciPath, req.CloudinitContent, 0644); err != nil {
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: fmt.Sprintf("write cloud-init failed: %v", err),
			})
			return
		}
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
			Message: fmt.Sprintf("Wrote cloud-init config to %s", ciPath),
		})
	}

	// Download & verify checksum
	sendEvent(&pb.BuildEvent{
		Type:     pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED,
		StepName: "Download & Verify",
	})

	filePath := filepath.Join(cfg.ISOPath, img.Name)
	needsDownload := true

	if _, err := os.Stat(filePath); err == nil && img.ChecksumUrl != "" {
		filenameFromURL := filepath.Base(img.Url)
		expectedCS, algo, err := checksum.GetExpectedChecksum(img.ChecksumUrl, filenameFromURL)
		if err == nil {
			localCS, err := checksum.CalculateFileChecksum(filePath, algo)
			if err == nil && localCS == expectedCS {
				sendEvent(&pb.BuildEvent{
					Type:    pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
					Message: fmt.Sprintf("Checksum match (%s). Skipping download.", algo),
				})
				needsDownload = false
			}
		}
	} else if _, err := os.Stat(filePath); err == nil && img.ChecksumUrl == "" {
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
			Message: "File exists, no checksum URL. Skipping download.",
		})
		needsDownload = false
	}

	if needsDownload {
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
			Message: fmt.Sprintf("Downloading %s", img.Url),
		})

		lastProgress := time.Time{}
		err := download.File(ctx, img.Url, filePath, func(downloaded, total int64) {
			if time.Since(lastProgress) > 100*time.Millisecond || downloaded == total {
				lastProgress = time.Now()
				var progress float64
				if total > 0 {
					progress = float64(downloaded) / float64(total)
				}
				sendEvent(&pb.BuildEvent{
					Type:     pb.BuildEventType_BUILD_EVENT_TYPE_DOWNLOAD_PROGRESS,
					StepName: "Download & Verify",
					Progress: progress,
				})
			}
		})
		if err != nil {
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED,
				Message: fmt.Sprintf("download failed: %v", err),
			})
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: fmt.Sprintf("download failed: %v", err),
			})
			return
		}
	}

	sendEvent(&pb.BuildEvent{
		Type:     pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED,
		StepName: "Download & Verify",
	})

	// Copy to working file
	workFile := filepath.Join(cfg.WorkDir, "base.qcow2")
	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
			Message: fmt.Sprintf("create work dir: %v", err),
		})
		return
	}

	sendEvent(&pb.BuildEvent{
		Type:     pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED,
		StepName: "Copy Image",
	})

	if err := fileutil.CopyFile(filePath, workFile); err != nil {
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED,
			Message: fmt.Sprintf("copy image: %v", err),
		})
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
			Message: fmt.Sprintf("copy image: %v", err),
		})
		return
	}
	sendEvent(&pb.BuildEvent{
		Type:     pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED,
		StepName: "Copy Image",
	})

	// Execute each build step
	vars := map[string]string{
		"ID":       fmt.Sprintf("%d", img.Id),
		"Name":     img.Name,
		"Tags":     img.Tags,
		"Vendor":   img.Vendor,
		"FilePath": workFile,
	}

	for i, step := range steps {
		if ctx.Err() != nil {
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: "build cancelled",
			})
			return
		}

		sendEvent(&pb.BuildEvent{
			Type:      pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED,
			StepName:  step.Name,
			StepIndex: int32(i),
		})

		command := executor.SubstituteVars(step.Command, vars)

		// Log the actual command being run
		sendEvent(&pb.BuildEvent{
			Type:      pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
			StepName:  step.Name,
			StepIndex: int32(i),
			Message:   fmt.Sprintf("$ %s", command),
		})

		output := make(chan string, 100)
		drained := make(chan struct{})

		go func() {
			for line := range output {
				sendEvent(&pb.BuildEvent{
					Type:      pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
					StepName:  step.Name,
					StepIndex: int32(i),
					Message:   line,
				})
			}
			close(drained)
		}()

		err := executor.ExecuteCommand(ctx, command, output)
		close(output)
		<-drained // wait for all log lines to be forwarded

		if err != nil {
			sendEvent(&pb.BuildEvent{
				Type:      pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED,
				StepName:  step.Name,
				StepIndex: int32(i),
				Message:   err.Error(),
			})
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: fmt.Sprintf("step %q failed: %v", step.Name, err),
			})
			// Clean up
			os.Remove(workFile)
			return
		}

		sendEvent(&pb.BuildEvent{
			Type:      pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED,
			StepName:  step.Name,
			StepIndex: int32(i),
		})
	}

	// Clean up working file
	os.Remove(workFile)

	sendEvent(&pb.BuildEvent{
		Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED,
		Message: "build completed successfully",
	})
}
