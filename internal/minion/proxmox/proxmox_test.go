package proxmox

import (
	"context"
	"errors"
	"testing"
)

func TestReplacementGuards(t *testing.T) {
	inventory := `[{"vmid":100,"node":"node1","type":"qemu","template":1},{"vmid":101,"node":"node1","type":"qemu"},{"vmid":102,"node":"node2","type":"qemu"},{"vmid":103,"node":"node1","type":"lxc"}]`
	c := &Client{Node: "node1", Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "qm" {
			if args[1] == "100" {
				return []byte("template: 1"), nil
			}
			return []byte("name: guest"), nil
		}
		return []byte(inventory), nil
	}}
	for _, tc := range []struct {
		source, target         int32
		overwrite, ok, replace bool
	}{
		{100, 100, true, false, false}, {99, 104, false, false, false}, {999, 101, true, false, false}, {101, 104, false, false, false}, {100, 101, false, false, false}, {100, 102, true, false, false}, {100, 103, true, false, false}, {100, 104, false, true, false}, {100, 101, true, true, true},
	} {
		replace, err := c.CheckClone(context.Background(), tc.source, tc.target, tc.overwrite)
		if (err == nil) != tc.ok || replace != tc.replace {
			t.Errorf("%+v: replace=%v err=%v", tc, replace, err)
		}
	}
	if _, err := c.CheckBuild(context.Background(), 101, true); err == nil {
		t.Fatal("build allowed replacement of guest")
	}
	if _, err := c.CheckBuild(context.Background(), 100, false); err == nil {
		t.Fatal("build replaced without consent")
	}
	if replace, err := c.CheckBuild(context.Background(), 100, true); err != nil || !replace {
		t.Fatalf("template replacement: %v", err)
	}
	c.Run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("inventory unavailable")
	}
	if _, err := c.CheckClone(context.Background(), 100, 104, true); err == nil {
		t.Fatal("inventory error treated as empty")
	}
}
