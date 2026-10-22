// Package preflight inspects a node without changing VMs or storage configuration.
package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/minion/config"
	"github.com/aloks98/pve-ctgen/internal/minion/proxmox"
	"github.com/aloks98/pve-ctgen/internal/shared/checksum"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"golang.org/x/sys/unix"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func Check(ctx context.Context, cfg *config.Config, pve *proxmox.Client, req *pb.PreflightRequest) *pb.PreflightResponse {
	result := &pb.PreflightResponse{}
	add := func(name string, err error, ok string) {
		c := &pb.CheckResult{Name: name, Ok: err == nil, Message: ok}
		if err != nil {
			c.Message = err.Error()
		}
		result.Checks = append(result.Checks, c)
	}
	profile, err := cfg.Profile(req.Profile)
	add("profile", err, req.Profile)
	if err != nil {
		return result
	}
	for _, tool := range []string{"qm", "pvesh", "pvesm", "qemu-img", "bash"} {
		_, err := pve.Run(ctx, "/bin/sh", "-c", "command -v "+tool)
		add("tool "+tool, err, "available")
	}
	out, err := pve.Run(ctx, "pveversion")
	add("Proxmox", err, strings.TrimSpace(string(out)))
	for _, dir := range []string{cfg.ISOPath, cfg.WorkDir, cfg.JobsDir, profile.SnippetsPath} {
		st, err := os.Stat(dir)
		if err == nil && !st.IsDir() {
			err = fmt.Errorf("not a directory")
		}
		if err == nil {
			err = unix.Access(dir, unix.W_OK|unix.X_OK)
		}
		add("directory "+dir, err, "writable")
		if err == nil {
			var stat unix.Statfs_t
			err = unix.Statfs(dir, &stat)
			if err == nil && uint64(stat.Bavail)*uint64(stat.Bsize) < 1024*1024*1024 {
				err = fmt.Errorf("less than 1 GiB free")
			}
			add("free space "+dir, err, "at least 1 GiB")
		}
	}
	for _, item := range []struct{ name, content string }{{profile.Storage, "images"}, {profile.SnippetStorage, "snippets"}} {
		out, err := pve.Run(ctx, "pvesh", "get", "/storage/"+item.name, "--output-format", "json")
		var storage struct {
			Content, Path string
			Disable       int
		}
		if err == nil {
			err = json.Unmarshal(out, &storage)
		}
		if err == nil && storage.Disable != 0 {
			err = fmt.Errorf("storage disabled")
		}
		if err == nil && !hasContent(storage.Content, item.content) {
			err = fmt.Errorf("storage %s must support %s", item.name, item.content)
		}
		if err == nil && item.content == "snippets" && filepath.Clean(filepath.Join(storage.Path, "snippets")) != filepath.Clean(profile.SnippetsPath) {
			err = fmt.Errorf("snippets_path must match storage directory %s/snippets", storage.Path)
		}
		if err == nil {
			out, err = pve.Run(ctx, "pvesh", "get", "/nodes/"+cfg.NodeName+"/storage/"+item.name+"/status", "--output-format", "json")
			var live struct {
				Active int    `json:"active"`
				Avail  uint64 `json:"avail"`
			}
			if err == nil {
				err = json.Unmarshal(out, &live)
			}
			if err == nil && live.Active != 1 {
				err = fmt.Errorf("storage not active on this node")
			}
			if err == nil && item.content == "images" {
				need, _ := config.DiskBytes(profile.DiskSize)
				if live.Avail < need {
					err = fmt.Errorf("storage has %d bytes free; profile disk needs %d", live.Avail, need)
				}
			}
		}
		add("storage "+item.name+" ("+item.content+")", err, "active and supports requested content")
	}
	_, err = pve.Run(ctx, "ip", "link", "show", "dev", profile.Bridge)
	add("bridge "+profile.Bridge, err, "present")
	if req.Build != nil {
		err := validation.Build(req.Build)
		add("build request", err, "valid")
		if err == nil {
			_, err = pve.CheckBuild(ctx, req.Build.Image.Id, req.Build.Options["overwrite"] == "true")
			add("target VM ID", err, "safe")
			u, _ := url.Parse(req.Build.Image.Url)
			_, algo, err := checksum.GetExpectedChecksumContext(ctx, req.Build.Image.ChecksumUrl, filepath.Base(u.Path))
			if err == nil && algo != "sha256" && algo != "sha512" {
				err = fmt.Errorf("SHA-256 or SHA-512 required")
			}
			add("image checksum", err, algo)
		}
	}
	if req.Launch != nil {
		err := validation.Launch(req.Launch)
		add("launch request", err, "valid")
		if err == nil {
			_, err = pve.CheckClone(ctx, req.Launch.TemplateId, req.Launch.NewVmId, req.Launch.Overwrite)
			add("clone source and target", err, "safe")
		}
	}
	return result
}
func hasContent(contents, want string) bool {
	for _, s := range strings.Split(contents, ",") {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}
func Error(r *pb.PreflightResponse) error {
	var failures []string
	for _, c := range r.Checks {
		if !c.Ok {
			failures = append(failures, c.Name+": "+c.Message)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("preflight failed: %s", strings.Join(failures, "; "))
	}
	return nil
}
