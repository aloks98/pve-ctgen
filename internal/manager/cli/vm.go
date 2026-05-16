package cli

import (
	"context"
	"fmt"
	"time"

	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/spf13/cobra"
)

func newVMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "VM operations",
	}

	cmd.AddCommand(
		newVMLaunchCmd(),
		newVMListCmd(),
	)

	return cmd
}

func newVMLaunchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launch",
		Short: "Launch a VM from a template",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			templateID, _ := cmd.Flags().GetInt("template-id")
			vmID, _ := cmd.Flags().GetInt("vm-id")
			name, _ := cmd.Flags().GetString("name")
			nodeName, _ := cmd.Flags().GetString("node")
			start, _ := cmd.Flags().GetBool("start")
			startAtBoot, _ := cmd.Flags().GetBool("start-at-boot")
			ipConfig, _ := cmd.Flags().GetString("ip")
			memory, _ := cmd.Flags().GetInt("memory")
			cores, _ := cmd.Flags().GetInt("cores")
			overwrite, _ := cmd.Flags().GetBool("overwrite")

			node, err := db.GetNode(nodeName)
			if err != nil {
				return fmt.Errorf("node %q: %w", nodeName, err)
			}

			client, err := managergrpc.NewClient(node.Address, node.APIKey)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Detect init type from local store by VM ID
			initType := "cloudinit"
			if templates, err := db.ListTemplates(); err == nil {
				for _, t := range templates {
					if t.VMID == templateID {
						initType = t.EffectiveInitType()
						break
					}
				}
			}

			resp, err := client.LaunchVM(ctx, &pb.LaunchVMRequest{
				TemplateId:  int32(templateID),
				NewVmId:     int32(vmID),
				Name:        name,
				Start:       start,
				StartAtBoot: startAtBoot,
				IpConfig:    ipConfig,
				Memory:      int32(memory),
				Cores:       int32(cores),
				InitType:    initType,
				Overwrite:   overwrite,
			})
			if err != nil {
				return fmt.Errorf("launch VM: %w", err)
			}

			if resp.Success {
				fmt.Printf("VM %d created: %s\n", resp.VmId, resp.Message)
			} else {
				fmt.Printf("Failed: %s\n", resp.Message)
			}
			return nil
		},
	}
	cmd.Flags().Int("template-id", 0, "Template VM ID to clone from")
	cmd.Flags().Int("vm-id", 0, "New VM ID")
	cmd.Flags().String("name", "", "VM name")
	cmd.Flags().String("node", "", "Target node")
	cmd.Flags().Bool("start", false, "Start VM after creation")
	cmd.Flags().Bool("start-at-boot", false, "Start VM at boot")
	cmd.Flags().String("ip", "dhcp", "IP configuration")
	cmd.Flags().Int("memory", 2048, "Memory in MB")
	cmd.Flags().Int("cores", 2, "CPU cores")
	cmd.Flags().Bool("overwrite", false, "Destroy and replace if the VM ID already exists")
	_ = cmd.MarkFlagRequired("template-id")
	_ = cmd.MarkFlagRequired("vm-id")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("node")
	return cmd
}

func newVMListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List VMs on a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			nodeName, _ := cmd.Flags().GetString("node")
			node, err := db.GetNode(nodeName)
			if err != nil {
				return fmt.Errorf("node %q: %w", nodeName, err)
			}

			client, err := managergrpc.NewClient(node.Address, node.APIKey)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.ListTemplates(ctx)
			if err != nil {
				return fmt.Errorf("list templates: %w", err)
			}

			if len(resp.Templates) == 0 {
				fmt.Println("No templates found on node.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "VM_ID\tNAME\tTAGS")
			for _, t := range resp.Templates {
				fmt.Fprintf(w, "%d\t%s\t%s\n", t.VmId, t.Name, t.Tags)
			}
			return w.Flush()
		},
	}
	cmd.Flags().String("node", "", "Node name")
	_ = cmd.MarkFlagRequired("node")
	return cmd
}
