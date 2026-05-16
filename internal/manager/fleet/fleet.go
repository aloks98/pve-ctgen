// Package fleet defines the declarative fleet manifest and resolves it into
// a concrete, ordered list of VMs to launch across one or more nodes.
package fleet

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Spec is a declarative description of a fleet of VMs.
type Spec struct {
	Name     string   `yaml:"name"`
	Defaults Settings `yaml:"defaults"`
	Groups   []Group  `yaml:"groups"`
}

// Settings are launch parameters that can be set fleet-wide as defaults and
// overridden per group. Pointer/zero values mean "inherit the default".
type Settings struct {
	Template     string `yaml:"template,omitempty"`
	Cores        int32  `yaml:"cores,omitempty"`
	Memory       int32  `yaml:"memory,omitempty"`
	IP           string `yaml:"ip,omitempty"` // count-mode ipconfig0; default "ip=dhcp"
	CIDR         string `yaml:"cidr,omitempty"`     // prefix for hosts-mode bare IPs, e.g. "16"
	Gateway      string `yaml:"gateway,omitempty"`  // gw for hosts-mode bare IPs
	Nameserver   string `yaml:"nameserver,omitempty"`
	SearchDomain string `yaml:"search_domain,omitempty"`
	Start        *bool  `yaml:"start,omitempty"`
	StartAtBoot  *bool  `yaml:"start_at_boot,omitempty"`
	Overwrite    *bool  `yaml:"overwrite,omitempty"`
}

// Host is one explicitly-specified VM (used when a group lists `hosts:`
// instead of using count-based expansion).
type Host struct {
	Node     string `yaml:"node"`
	VMID     int32  `yaml:"vmid"`
	Hostname string `yaml:"hostname"`
	IP       string `yaml:"ip"` // bare addr (uses group cidr/gateway) or full ipconfig0
}

// Group is one set of VMs (e.g. "control-plane", "worker"). It is either
// count-based (count + vmid_start + node/nodes + spread + hostname) or
// explicit (hosts: [...]). Settings is inlined so any default can be
// overridden directly on the group.
type Group struct {
	Role      string   `yaml:"role"`
	Count     int      `yaml:"count,omitempty"`
	Node      string   `yaml:"node,omitempty"`  // single target node
	Nodes     []string `yaml:"nodes,omitempty"` // multi-node target set
	Spread    string   `yaml:"spread,omitempty"`
	VMIDStart int32    `yaml:"vmid_start,omitempty"`
	Hostname  string   `yaml:"hostname,omitempty"`
	Hosts     []Host   `yaml:"hosts,omitempty"`
	Settings  `yaml:",inline"`
}

// PlannedVM is a single resolved VM ready to be launched.
type PlannedVM struct {
	Role         string
	Node         string
	VMID         int32
	Name         string
	Hostname     string
	Template     string
	Cores        int32
	Memory       int32
	IP           string
	Nameserver   string
	SearchDomain string
	Start        bool
	StartAtBoot  bool
	Overwrite    bool
}

// Spread strategies for distributing a group across multiple nodes.
const (
	SpreadRoundRobin = "round-robin" // item i -> nodes[i % len(nodes)] (default)
	SpreadFill       = "fill"        // contiguous even chunks per node
)

// Load reads and parses a fleet manifest from disk.
func Load(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fleet manifest: %w", err)
	}
	var s Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse fleet manifest: %w", err)
	}
	if len(s.Groups) == 0 {
		return nil, fmt.Errorf("fleet manifest has no groups")
	}
	return &s, nil
}

// merge returns d with any explicitly-set fields from o applied on top.
func (d Settings) merge(o Settings) Settings {
	r := d
	if o.Template != "" {
		r.Template = o.Template
	}
	if o.Cores != 0 {
		r.Cores = o.Cores
	}
	if o.Memory != 0 {
		r.Memory = o.Memory
	}
	if o.IP != "" {
		r.IP = o.IP
	}
	if o.CIDR != "" {
		r.CIDR = o.CIDR
	}
	if o.Gateway != "" {
		r.Gateway = o.Gateway
	}
	if o.Nameserver != "" {
		r.Nameserver = o.Nameserver
	}
	if o.SearchDomain != "" {
		r.SearchDomain = o.SearchDomain
	}
	if o.Start != nil {
		r.Start = o.Start
	}
	if o.StartAtBoot != nil {
		r.StartAtBoot = o.StartAtBoot
	}
	if o.Overwrite != nil {
		r.Overwrite = o.Overwrite
	}
	return r
}

func deref(b *bool) bool { return b != nil && *b }

func (g Group) nodeList() ([]string, error) {
	switch {
	case g.Node != "" && len(g.Nodes) > 0:
		return nil, fmt.Errorf("set either `node` or `nodes`, not both")
	case g.Node != "":
		return []string{g.Node}, nil
	case len(g.Nodes) > 0:
		return g.Nodes, nil
	default:
		return nil, fmt.Errorf("no target node(s): set `node` or `nodes`")
	}
}

// assignNode picks the node for the i-th of count items across nodes.
func assignNode(nodes []string, spread string, i, count int) string {
	if len(nodes) == 1 {
		return nodes[0]
	}
	if spread == SpreadFill {
		per := (count + len(nodes) - 1) / len(nodes) // ceil
		idx := i / per
		if idx >= len(nodes) {
			idx = len(nodes) - 1
		}
		return nodes[idx]
	}
	return nodes[i%len(nodes)] // round-robin default
}

// buildIPConfig turns a host `ip` value into a Proxmox ipconfig0 string.
// A value containing '=' or ',' is treated as a full ipconfig0 and passed
// through. A bare address gets cidr/gateway applied: "ip=<addr>/<cidr>,gw=<gw>".
func buildIPConfig(ip, cidr, gateway string) (string, error) {
	if ip == "" {
		return "", fmt.Errorf("ip is required")
	}
	if strings.ContainsAny(ip, "=,") {
		return ip, nil // full ipconfig0 passthrough
	}
	addr := ip
	if !strings.Contains(addr, "/") {
		if cidr == "" {
			return "", fmt.Errorf("ip %q has no prefix and no `cidr` set (in defaults or group)", ip)
		}
		addr += "/" + strings.TrimPrefix(cidr, "/")
	}
	out := "ip=" + addr
	if gateway != "" {
		out += ",gw=" + gateway
	}
	return out, nil
}

// Resolve expands the spec into the concrete, ordered list of VMs to launch.
// It validates required fields and that VM IDs do not collide across groups.
func (s *Spec) Resolve() ([]PlannedVM, error) {
	var plan []PlannedVM
	seen := map[int32]string{}

	for gi, g := range s.Groups {
		label := g.Role
		if label == "" {
			label = fmt.Sprintf("group[%d]", gi)
		}
		eff := s.Defaults.merge(g.Settings)
		if eff.Template == "" {
			return nil, fmt.Errorf("%s: template is required (set in defaults or on the group)", label)
		}

		// Explicit hosts mode.
		if len(g.Hosts) > 0 {
			if g.Count != 0 || g.Node != "" || len(g.Nodes) > 0 || g.VMIDStart != 0 || g.Hostname != "" || g.Spread != "" {
				return nil, fmt.Errorf("%s: use either `hosts:` or count-based fields, not both", label)
			}
			for hi, h := range g.Hosts {
				where := fmt.Sprintf("%s host[%d]", label, hi)
				if h.Node == "" {
					return nil, fmt.Errorf("%s: node is required", where)
				}
				if h.VMID <= 0 {
					return nil, fmt.Errorf("%s: vmid must be > 0", where)
				}
				if h.Hostname == "" {
					return nil, fmt.Errorf("%s: hostname is required", where)
				}
				ipc, err := buildIPConfig(h.IP, eff.CIDR, eff.Gateway)
				if err != nil {
					return nil, fmt.Errorf("%s (%s): %w", where, h.Hostname, err)
				}
				if prev, dup := seen[h.VMID]; dup {
					return nil, fmt.Errorf("VM ID %d assigned to both %s and %s", h.VMID, prev, h.Hostname)
				}
				seen[h.VMID] = h.Hostname
				plan = append(plan, PlannedVM{
					Role:         label,
					Node:         h.Node,
					VMID:         h.VMID,
					Name:         h.Hostname,
					Hostname:     h.Hostname,
					Template:     eff.Template,
					Cores:        eff.Cores,
					Memory:       eff.Memory,
					IP:           ipc,
					Nameserver:   eff.Nameserver,
					SearchDomain: eff.SearchDomain,
					Start:        deref(eff.Start),
					StartAtBoot:  deref(eff.StartAtBoot),
					Overwrite:    deref(eff.Overwrite),
				})
			}
			continue
		}

		// Count-based mode.
		if g.Count <= 0 {
			return nil, fmt.Errorf("%s: count must be > 0 (or use `hosts:`)", label)
		}
		if g.VMIDStart <= 0 {
			return nil, fmt.Errorf("%s: vmid_start must be set", label)
		}
		if g.Hostname == "" {
			return nil, fmt.Errorf("%s: hostname is required", label)
		}
		if g.Spread != "" && g.Spread != SpreadRoundRobin && g.Spread != SpreadFill {
			return nil, fmt.Errorf("%s: unknown spread %q (use %q or %q)", label, g.Spread, SpreadRoundRobin, SpreadFill)
		}
		nodes, err := g.nodeList()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		ip := eff.IP
		if ip == "" {
			ip = "ip=dhcp"
		}
		for i := range g.Count {
			vmid := g.VMIDStart + int32(i)
			tag := fmt.Sprintf("%s#%d", label, i+1)
			if prev, dup := seen[vmid]; dup {
				return nil, fmt.Errorf("VM ID %d assigned to both %s and %s", vmid, prev, tag)
			}
			seen[vmid] = tag

			name := g.Hostname
			if g.Count > 1 {
				name = fmt.Sprintf("%s-%d", g.Hostname, i+1)
			}
			plan = append(plan, PlannedVM{
				Role:         label,
				Node:         assignNode(nodes, g.Spread, i, g.Count),
				VMID:         vmid,
				Name:         name,
				Hostname:     name,
				Template:     eff.Template,
				Cores:        eff.Cores,
				Memory:       eff.Memory,
				IP:           ip,
				Nameserver:   eff.Nameserver,
				SearchDomain: eff.SearchDomain,
				Start:        deref(eff.Start),
				StartAtBoot:  deref(eff.StartAtBoot),
				Overwrite:    deref(eff.Overwrite),
			})
		}
	}
	return plan, nil
}
