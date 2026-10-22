package validation

import (
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	ignitionconfig "github.com/coreos/ignition/v2/config/v3_6"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var fileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

func Name(s string) error {
	if !fileName.MatchString(s) || s == "." || s == ".." || filepath.Base(s) != s {
		return fmt.Errorf("invalid name %q: use letters, digits, dots, underscores and hyphens", s)
	}
	return nil
}
func Hostname(s string) error {
	if len(s) == 0 || len(s) > 253 {
		return fmt.Errorf("invalid hostname")
	}
	for _, p := range strings.Split(s, ".") {
		if len(p) > 63 || !hostLabel.MatchString(p) {
			return fmt.Errorf("invalid hostname %q", s)
		}
	}
	return nil
}
func URL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("URL must use HTTP(S), have a host, and contain no credentials")
	}
	return nil
}
func IPConfig(s string) error {
	if s == "" {
		return nil
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || seen[k] {
			return fmt.Errorf("invalid IP configuration %q", s)
		}
		seen[k] = true
		switch k {
		case "ip", "ip6":
			if v == "dhcp" || (k == "ip6" && v == "auto") {
				continue
			}
			p, err := netip.ParsePrefix(v)
			if err != nil || (k == "ip" && !p.Addr().Is4()) || (k == "ip6" && !p.Addr().Is6()) {
				return fmt.Errorf("invalid %s prefix", k)
			}
		case "gw", "gw6":
			a, err := netip.ParseAddr(v)
			if err != nil || (k == "gw" && !a.Is4()) || (k == "gw6" && !a.Is6()) {
				return fmt.Errorf("invalid %s address", k)
			}
		default:
			return fmt.Errorf("unknown IP configuration field %q", k)
		}
	}
	return nil
}
func Build(r *pb.BuildRequest) error {
	if r == nil || r.Image == nil {
		return fmt.Errorf("image is required")
	}
	if err := Name(r.BuildId); err != nil {
		return fmt.Errorf("build ID: %w", err)
	}
	if r.Image.Id < 100 || r.Image.Id > 999999999 {
		return fmt.Errorf("VM ID must be between 100 and 999999999")
	}
	if err := Name(r.Image.Name); err != nil {
		return err
	}
	if err := URL(r.Image.Url); err != nil {
		return err
	}
	if err := URL(r.Image.ChecksumUrl); err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	if r.InitType != "" && r.InitType != "cloudinit" && r.InitType != "ignition" {
		return fmt.Errorf("unknown init type %q", r.InitType)
	}
	if (len(r.CloudinitContent) > 0) != (r.CloudinitFilename != "") {
		return fmt.Errorf("init content and filename must be provided together")
	}
	if r.CloudinitFilename != "" {
		if err := Name(r.CloudinitFilename); err != nil {
			return err
		}
		if r.InitType == "ignition" {
			_, report, err := ignitionconfig.ParseCompatibleVersion(r.CloudinitContent)
			if err != nil || report.IsFatal() {
				return fmt.Errorf("invalid Ignition configuration: %v %s", err, report.String())
			}
		} else if err := cloudinit.Validate(string(r.CloudinitContent)); err != nil {
			return err
		}
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("at least one build step is required")
	}
	seen := map[string]bool{"Download & Verify": true, "Copy Image": true}
	for _, s := range r.Steps {
		if s == nil || strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.Command) == "" {
			return fmt.Errorf("each step needs a name and command")
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate or reserved step name %q", s.Name)
		}
		seen[s.Name] = true
	}
	for k, v := range r.Options {
		if k == "overwrite" {
			if v != "true" && v != "false" {
				return fmt.Errorf("overwrite must be true or false")
			}
		} else if k != "profile" {
			return fmt.Errorf("unknown build option %q", k)
		}
	}
	return nil
}
func Launch(r *pb.LaunchVMRequest) error {
	if r == nil {
		return fmt.Errorf("launch request is required")
	}
	if r.TemplateId < 100 || r.NewVmId < 100 || r.TemplateId > 999999999 || r.NewVmId > 999999999 || r.TemplateId == r.NewVmId {
		return fmt.Errorf("source and destination must be distinct valid VM IDs")
	}
	if err := Hostname(r.Name); err != nil {
		return err
	}
	if r.Hostname != "" {
		if err := Hostname(r.Hostname); err != nil {
			return err
		}
	}
	if r.Memory < 0 || r.Cores < 0 || r.Cores > 8192 {
		return fmt.Errorf("invalid memory or core count")
	}
	if r.InitType != "" && r.InitType != "cloudinit" && r.InitType != "ignition" {
		return fmt.Errorf("unknown init type")
	}
	if r.SearchDomain != "" {
		if err := Hostname(r.SearchDomain); err != nil {
			return err
		}
	}
	for _, s := range strings.Fields(r.Nameserver) {
		if _, err := netip.ParseAddr(s); err != nil {
			return fmt.Errorf("invalid nameserver %q", s)
		}
	}
	return IPConfig(r.IpConfig)
}
