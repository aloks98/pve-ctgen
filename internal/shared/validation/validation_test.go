package validation

import (
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"testing"
)

func TestRejectUnsafeRequests(t *testing.T) {
	for _, name := range []string{"../outside", "/etc/passwd", "a/b", "..", "a\ncommand"} {
		if Name(name) == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, s := range []string{"dhcp", "ip=not-an-ip", "ip=10.0.0.1/99", "ip=dhcp,ip=dhcp", "ip6=10.0.0.1/24"} {
		if IPConfig(s) == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if IPConfig("ip=10.0.0.2/24,gw=10.0.0.1") != nil {
		t.Fatal("valid static IP rejected")
	}
	if Build(nil) == nil || Build(&pb.BuildRequest{}) == nil {
		t.Fatal("nil image accepted")
	}
	if Launch(&pb.LaunchVMRequest{TemplateId: 100, NewVmId: 101, Name: "test", Hostname: "x\nmalicious: value"}) == nil {
		t.Fatal("invalid hostname accepted")
	}
}
