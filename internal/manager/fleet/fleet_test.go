package fleet

import "testing"

func b(v bool) *bool { return &v }

func TestResolveSpreadAndDefaults(t *testing.T) {
	s := &Spec{
		Name: "k8s",
		Defaults: Settings{
			Template:  "flatcar-k8s",
			Cores:     4,
			Memory:    4096,
			Start:     b(true),
			Overwrite: b(false),
		},
		Groups: []Group{
			{Role: "control-plane", Count: 3, Node: "rack-1", VMIDStart: 100, Hostname: "k8s-cp"},
			{
				Role: "worker", Count: 5,
				Nodes: []string{"rack-1", "rack-2", "rack-3"}, Spread: SpreadRoundRobin,
				VMIDStart: 110, Hostname: "k8s-worker",
				Settings: Settings{Memory: 8192, Overwrite: b(true)},
			},
		},
	}
	plan, err := s.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(plan) != 8 {
		t.Fatalf("want 8 VMs, got %d", len(plan))
	}

	// Control plane: all on rack-1, IDs 100-102, defaults applied.
	for i := range 3 {
		p := plan[i]
		if p.Node != "rack-1" || p.VMID != int32(100+i) {
			t.Errorf("cp[%d]: node=%s vmid=%d", i, p.Node, p.VMID)
		}
		if p.Name != []string{"k8s-cp-1", "k8s-cp-2", "k8s-cp-3"}[i] {
			t.Errorf("cp[%d]: name=%s", i, p.Name)
		}
		if p.Memory != 4096 || p.Cores != 4 || !p.Start || p.Overwrite {
			t.Errorf("cp[%d]: defaults not applied: %+v", i, p)
		}
	}

	// Workers: round-robin rack-1,2,3,1,2 ; IDs 110-114 ; group overrides.
	wantNodes := []string{"rack-1", "rack-2", "rack-3", "rack-1", "rack-2"}
	for i := range 5 {
		p := plan[3+i]
		if p.Node != wantNodes[i] {
			t.Errorf("worker[%d]: node=%s want %s", i, p.Node, wantNodes[i])
		}
		if p.VMID != int32(110+i) {
			t.Errorf("worker[%d]: vmid=%d", i, p.VMID)
		}
		if p.Memory != 8192 { // group override
			t.Errorf("worker[%d]: memory=%d want 8192", i, p.Memory)
		}
		if !p.Overwrite { // group override
			t.Errorf("worker[%d]: overwrite not overridden", i)
		}
		if p.IP != "ip=dhcp" {
			t.Errorf("worker[%d]: ip=%q want default ip=dhcp", i, p.IP)
		}
	}
}

func TestResolveFillSpread(t *testing.T) {
	s := &Spec{
		Groups: []Group{{
			Role: "w", Count: 5, Nodes: []string{"a", "b"}, Spread: SpreadFill,
			VMIDStart: 200, Hostname: "w", Settings: Settings{Template: "t"},
		}},
	}
	plan, err := s.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// ceil(5/2)=3 -> a,a,a,b,b
	want := []string{"a", "a", "a", "b", "b"}
	for i, p := range plan {
		if p.Node != want[i] {
			t.Errorf("[%d] node=%s want %s", i, p.Node, want[i])
		}
	}
}

func TestResolveSingleCountNoSuffix(t *testing.T) {
	s := &Spec{Groups: []Group{{
		Role: "lb", Count: 1, Node: "n", VMIDStart: 300, Hostname: "haproxy",
		Settings: Settings{Template: "debian"},
	}}}
	plan, _ := s.Resolve()
	if plan[0].Name != "haproxy" {
		t.Errorf("count=1 should not suffix, got %q", plan[0].Name)
	}
}

func TestResolveErrors(t *testing.T) {
	cases := map[string]*Spec{
		"vmid collision": {Groups: []Group{
			{Role: "a", Count: 2, Node: "n", VMIDStart: 100, Hostname: "a", Settings: Settings{Template: "t"}},
			{Role: "b", Count: 2, Node: "n", VMIDStart: 101, Hostname: "b", Settings: Settings{Template: "t"}},
		}},
		"missing template": {Groups: []Group{
			{Role: "a", Count: 1, Node: "n", VMIDStart: 100, Hostname: "a"},
		}},
		"node and nodes": {Groups: []Group{
			{Role: "a", Count: 1, Node: "n", Nodes: []string{"m"}, VMIDStart: 100, Hostname: "a", Settings: Settings{Template: "t"}},
		}},
		"bad spread": {Groups: []Group{
			{Role: "a", Count: 1, Nodes: []string{"n"}, Spread: "wat", VMIDStart: 100, Hostname: "a", Settings: Settings{Template: "t"}},
		}},
	}
	for name, s := range cases {
		if _, err := s.Resolve(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
