package builder

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aloks98/pve-ctgen/internal/minion/executor"
	"github.com/aloks98/pve-ctgen/internal/minion/proxmox"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/download"
	"github.com/aloks98/pve-ctgen/internal/shared/filelock"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
)

// Config holds builder runtime paths.
type Config struct {
	Profile      models.BuildProfile
	NodeName     string
	ISOPath      string
	SnippetsPath string
	WorkDir      string
}

// RunBuild orchestrates a full build: download, checksum, cloud-init, execute steps.
// Events are sent on the events channel. The channel is closed when the build completes.
func RunBuild(ctx context.Context, req *pb.BuildRequest, cfg Config, events chan<- *pb.BuildEvent) {
	defer close(events)

	if err := validation.Build(req); err != nil {
		select {
		case events <- &pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()}:
		case <-ctx.Done():
		}
		return
	}

	buildID := req.BuildId
	img := req.Image
	if cfg.Profile.SnippetsPath != "" {
		cfg.SnippetsPath = cfg.Profile.SnippetsPath
	}
	steps := req.Steps

	sendEvent := func(e *pb.BuildEvent) {
		e.BuildId = buildID
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}

	releaseVM, err := filelock.Acquire(ctx, filepath.Join(cfg.WorkDir, "locks", fmt.Sprintf("vm-%d.lock", img.Id)))
	if err != nil {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()})
		return
	}
	defer releaseVM()
	workspace, err := os.MkdirTemp(cfg.WorkDir, "build-")
	if err != nil {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()})
		return
	}
	defer os.RemoveAll(workspace)

	pve := proxmox.New(cfg.NodeName)
	overwrite := req.Options["overwrite"] == "true"
	if _, err := pve.CheckBuild(ctx, img.Id, overwrite); err != nil {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()})
		return
	}

	// Write init config (cloud-init YAML or Ignition JSON).
	// For ignition, the content is already transpiled JSON; we skip YAML validation.
	if len(req.CloudinitContent) > 0 && req.CloudinitFilename != "" {
		isIgnition := req.InitType == "ignition"
		if !isIgnition {
			if err := cloudinit.Validate(string(req.CloudinitContent)); err != nil {
				sendEvent(&pb.BuildEvent{
					Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
					Message: fmt.Sprintf("invalid cloud-init YAML: %v", err),
				})
				return
			}
		}
		snippetName := buildID + "-" + req.CloudinitFilename
		ciPath := filepath.Join(cfg.SnippetsPath, snippetName)
		if err := fileutil.AtomicWrite(ciPath, req.CloudinitContent, 0600); err != nil {
			sendEvent(&pb.BuildEvent{
				Type:    pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED,
				Message: fmt.Sprintf("write init config failed: %v", err),
			})
			return
		}
		label := "cloud-init"
		if isIgnition {
			label = "ignition"
		}
		sendEvent(&pb.BuildEvent{
			Type:    pb.BuildEventType_BUILD_EVENT_TYPE_LOG,
			Message: fmt.Sprintf("Wrote %s config to %s", label, ciPath),
		})
	}

	// Download & verify checksum
	sendEvent(&pb.BuildEvent{
		Type:     pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED,
		StepName: "Download & Verify",
	})

	cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(img.Url)))
	filePath := filepath.Join(cfg.ISOPath, cacheKey)
	releaseCache, err := filelock.Acquire(ctx, filepath.Join(cfg.WorkDir, "locks", "image-"+cacheKey+".lock"))
	if err != nil {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()})
		return
	}
	defer releaseCache()
	{

		lastProgress := time.Time{}
		err := download.Verified(ctx, img.Url, img.ChecksumUrl, filePath, func(downloaded, total int64) {
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
	workFile := filepath.Join(workspace, "base.qcow2")
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

	releaseCache()
	// Execute each build step
	vars := map[string]string{
		"ID":             fmt.Sprintf("%d", img.Id),
		"Name":           img.Name,
		"Tags":           img.Tags,
		"Vendor":         "",
		"Storage":        cfg.Profile.Storage,
		"SnippetStorage": cfg.Profile.SnippetStorage,
		"Bridge":         cfg.Profile.Bridge,
		"DiskSize":       cfg.Profile.DiskSize,
		"DiskFormat":     cfg.Profile.DiskFormat,
		"FilePath":       workFile,
	}

	if req.CloudinitFilename != "" {
		vars["Vendor"] = buildID + "-" + req.CloudinitFilename
	}
	// Reject images with host-file backing references before any VM mutation.
	info, err := pve.Run(ctx, "qemu-img", "info", "--output=json", workFile)
	var imageInfo struct {
		Format  string `json:"format"`
		Backing string `json:"backing-filename"`
	}
	if err == nil {
		err = json.Unmarshal(info, &imageInfo)
	}
	if err != nil || imageInfo.Backing != "" || (imageInfo.Format != "raw" && imageInfo.Format != "qcow2") {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: "unsupported image format or backing file"})
		return
	}

	// Recheck immediately before replacement, after download and validation.
	replace, err := pve.CheckBuild(ctx, img.Id, overwrite)
	if err == nil && replace {
		err = pve.Destroy(ctx, img.Id, false)
	}
	if err != nil {
		sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()})
		return
	}

	for i, step := range steps {
		// Legacy default steps are superseded by guarded replacement above.
		if step.Name == "Destroy VM if exists" {
			continue
		}
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

		if strings.Contains(step.Command, "{{.ImportedDisk}}") {
			vmcfg, err := pve.Config(ctx, img.Id)
			if err != nil || vmcfg["unused0"] == "" {
				sendEvent(&pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: "imported disk not found in unused0"})
				return
			}
			vars["ImportedDisk"] = strings.SplitN(vmcfg["unused0"], ",", 2)[0]
		}
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
