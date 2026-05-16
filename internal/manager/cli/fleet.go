package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aloks98/pve-ctgen/internal/manager/fleet"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/spf13/cobra"
)

func newFleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Bulk VM management from a declarative manifest",
		Long:  "Create many VMs across one or more nodes from a version-controllable YAML manifest. Re-running converges (existing VM IDs are skipped unless the manifest sets overwrite: true).",
	}
	cmd.AddCommand(newFleetApplyCmd())
	return cmd
}

type fleetResult struct {
	vm     fleet.PlannedVM
	status string // "created" | "skipped" | "failed"
	msg    string
}

func newFleetApplyCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply <fleet.yaml>",
		Short: "Create the fleet described by the manifest across nodes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := fleet.Load(args[0])
			if err != nil {
				return err
			}
			plan, err := spec.Resolve()
			if err != nil {
				return err
			}

			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			// Group the plan by node, preserving first-seen order.
			byNode := map[string][]fleet.PlannedVM{}
			var order []string
			for _, p := range plan {
				if _, seen := byNode[p.Node]; !seen {
					order = append(order, p.Node)
				}
				byNode[p.Node] = append(byNode[p.Node], p)
			}

			printPlan(spec.Name, order, byNode)
			if dryRun {
				fmt.Println("\n(dry run — no VMs created)")
				return nil
			}

			// Pre-flight: every referenced node must exist in the store.
			nodes := map[string]*models.Node{}
			for _, nn := range order {
				n, err := db.GetNode(nn)
				if err != nil {
					return fmt.Errorf("fleet references unknown node %q (add it with `manager node add`): %w", nn, err)
				}
				nodes[nn] = n
			}
			fmt.Println()

			// Launch each node concurrently; items within a node run
			// sequentially to keep qm load and output sane.
			var (
				mu      sync.Mutex
				results = map[string][]fleetResult{}
				wg      sync.WaitGroup
			)
			for _, nn := range order {
				wg.Add(1)
				go func(nodeName string, items []fleet.PlannedVM) {
					defer wg.Done()
					res := launchNodeItems(nodes[nodeName], items)
					mu.Lock()
					results[nodeName] = res
					mu.Unlock()
				}(nn, byNode[nn])
			}
			wg.Wait()

			var created, skipped, failed int
			for _, nn := range order {
				fmt.Printf("%s\n", nn)
				for _, r := range results[nn] {
					switch r.status {
					case "created":
						created++
						fmt.Printf("  [OK] %-22s VM %d\n", r.vm.Name, r.vm.VMID)
					case "skipped":
						skipped++
						fmt.Printf("  [--] %-22s VM %d  (already exists)\n", r.vm.Name, r.vm.VMID)
					default:
						failed++
						fmt.Printf("  [!!] %-22s VM %d  %s\n", r.vm.Name, r.vm.VMID, r.msg)
					}
				}
			}
			fmt.Printf("\n%d created, %d skipped, %d failed\n", created, skipped, failed)
			if failed > 0 {
				return fmt.Errorf("%d VM(s) failed", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the resolved plan without creating any VMs")
	return cmd
}

// launchNodeItems sequentially launches all planned VMs targeted at one node.
func launchNodeItems(node *models.Node, items []fleet.PlannedVM) []fleetResult {
	out := make([]fleetResult, 0, len(items))

	client, err := managergrpc.NewClient(node.Address, node.APIKey)
	if err != nil {
		for _, p := range items {
			out = append(out, fleetResult{vm: p, status: "failed", msg: fmt.Sprintf("connect: %v", err)})
		}
		return out
	}
	defer client.Close()

	// Resolve template name -> (vm id, tags) once per node.
	tmplCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	tmplResp, terr := client.ListTemplates(tmplCtx)
	cancel()
	type tinfo struct {
		id   int32
		tags string
	}
	tmap := map[string]tinfo{}
	if terr == nil {
		for _, t := range tmplResp.Templates {
			tmap[t.Name] = tinfo{id: t.VmId, tags: t.Tags}
		}
	}

	for _, p := range items {
		if terr != nil {
			out = append(out, fleetResult{vm: p, status: "failed", msg: fmt.Sprintf("list templates: %v", terr)})
			continue
		}
		ti, ok := tmap[p.Template]
		if !ok {
			out = append(out, fleetResult{vm: p, status: "failed", msg: fmt.Sprintf("template %q not found on node", p.Template)})
			continue
		}
		initType := models.InitTypeCloudInit
		if models.HasTag(ti.tags, models.InitTypeIgnition) {
			initType = models.InitTypeIgnition
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		resp, err := client.LaunchVM(ctx, &pb.LaunchVMRequest{
			TemplateId:   ti.id,
			NewVmId:      p.VMID,
			Name:         p.Name,
			Hostname:     p.Hostname,
			Start:        p.Start,
			StartAtBoot:  p.StartAtBoot,
			IpConfig:     p.IP,
			Memory:       p.Memory,
			Cores:        p.Cores,
			Nameserver:   p.Nameserver,
			SearchDomain: p.SearchDomain,
			InitType:     initType,
			Overwrite:    p.Overwrite,
		})
		cancel()

		switch {
		case err != nil:
			out = append(out, fleetResult{vm: p, status: "failed", msg: err.Error()})
		case resp.Success:
			out = append(out, fleetResult{vm: p, status: "created"})
		case strings.Contains(resp.Message, "already exists"):
			// Idempotent: overwrite was false and the VM is already there.
			out = append(out, fleetResult{vm: p, status: "skipped"})
		default:
			out = append(out, fleetResult{vm: p, status: "failed", msg: resp.Message})
		}
	}
	return out
}

func printPlan(name string, order []string, byNode map[string][]fleet.PlannedVM) {
	if name != "" {
		fmt.Printf("Fleet: %s\n", name)
	}
	for _, nn := range order {
		fmt.Printf("%s\n", nn)
		for _, p := range byNode[nn] {
			ow := ""
			if p.Overwrite {
				ow = "  overwrite"
			}
			fmt.Printf("  VM %-5d %-22s %-16s [%s]%s\n", p.VMID, p.Name, p.Template, p.Role, ow)
		}
	}
}
