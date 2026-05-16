package cli

import (
	"strings"
	"testing"
)

func TestResolveTemplate(t *testing.T) {
	node := []nodeTemplate{
		{name: "flatcar-k8s-9109-cloudinit", id: 9109, tags: "flatcar-template,ignition,k8s"},
		{name: "ubuntu2404-9100-cloudinit", id: 9100, tags: "ubuntu"},
		{name: "debian13-9101-cloudinit", id: 9101, tags: "debian"},
	}

	// Build-convention prefix match.
	got, err := resolveTemplate(node, "flatcar-k8s")
	if err != nil {
		t.Fatalf("flatcar-k8s: %v", err)
	}
	if got.id != 9109 || !strings.Contains(got.tags, "ignition") {
		t.Errorf("got %+v", got)
	}

	// Exact match wins over nothing.
	got, err = resolveTemplate([]nodeTemplate{{name: "flatcar-k8s", id: 50}}, "flatcar-k8s")
	if err != nil || got.id != 50 {
		t.Errorf("exact: got %+v err %v", got, err)
	}

	// Exact preferred over a convention sibling.
	got, err = resolveTemplate([]nodeTemplate{
		{name: "flatcar-k8s", id: 1},
		{name: "flatcar-k8s-9109-cloudinit", id: 9109},
	}, "flatcar-k8s")
	if err != nil || got.id != 1 {
		t.Errorf("exact-preferred: got %+v err %v", got, err)
	}

	// Not found lists what's available.
	_, err = resolveTemplate(node, "rocky10")
	if err == nil || !strings.Contains(err.Error(), "available:") {
		t.Errorf("not-found err = %v", err)
	}

	// Ambiguous: two convention matches, no exact.
	_, err = resolveTemplate([]nodeTemplate{
		{name: "flatcar-k8s-100-cloudinit", id: 100},
		{name: "flatcar-k8s-200-cloudinit", id: 200},
	}, "flatcar-k8s")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous err = %v", err)
	}

	// Prefix must be "<want>-"; "flatcar-k8s2" should not match "flatcar-k8s".
	_, err = resolveTemplate([]nodeTemplate{{name: "flatcar-k8s2-1-cloudinit", id: 9}}, "flatcar-k8s")
	if err == nil {
		t.Errorf("flatcar-k8s2 should not match flatcar-k8s")
	}
}
