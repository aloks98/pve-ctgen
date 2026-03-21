package cli

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/spf13/cobra"
)

func newTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage VM templates",
	}

	cmd.AddCommand(
		newTemplateListCmd(),
		newTemplateAddCmd(),
		newTemplateShowCmd(),
		newTemplateEditCmd(),
		newTemplateRemoveCmd(),
	)

	return cmd
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			templates, err := db.ListTemplates()
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				fmt.Println("No templates found. Run 'pvectgen manager import' to import from files.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "VM_ID\tNAME\tURL\tTAGS\tCLOUDINIT")
			for _, t := range templates {
				ci := "-"
				if t.CloudInit != "" {
					ci = t.CloudInit
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.VMID, t.Name, t.URL, t.Tags, ci)
			}
			return w.Flush()
		},
	}
}

func newTemplateAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new template",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			vmID, _ := cmd.Flags().GetInt("vm-id")
			name, _ := cmd.Flags().GetString("name")
			url, _ := cmd.Flags().GetString("url")
			checksumURL, _ := cmd.Flags().GetString("checksum-url")
			tags, _ := cmd.Flags().GetString("tags")
			cloudinit, _ := cmd.Flags().GetString("cloudinit")

			t := &models.Template{
				VMID:        vmID,
				Name:        name,
				URL:         url,
				ChecksumURL: checksumURL,
				Tags:        tags,
			}

			if cloudinit != "" {
				if _, err := db.GetCloudInit(cloudinit); err != nil {
					return fmt.Errorf("cloud-init config %q not found", cloudinit)
				}
				t.CloudInit = cloudinit
			}

			if _, err := db.CreateTemplate(t); err != nil {
				return err
			}
			fmt.Printf("Added template %q (VM ID: %d)\n", name, vmID)
			return nil
		},
	}
	cmd.Flags().Int("vm-id", 0, "Proxmox VM ID")
	cmd.Flags().String("name", "", "Template name")
	cmd.Flags().String("url", "", "Image download URL")
	cmd.Flags().String("checksum-url", "", "Checksum file URL")
	cmd.Flags().String("tags", "", "Comma-separated tags")
	cmd.Flags().String("cloudinit", "", "Cloud-init config name")
	_ = cmd.MarkFlagRequired("vm-id")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newTemplateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show template details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			t, err := db.GetTemplate(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:         %s\n", t.Name)
			fmt.Printf("VM ID:        %d\n", t.VMID)
			fmt.Printf("URL:          %s\n", t.URL)
			fmt.Printf("Checksum URL: %s\n", t.ChecksumURL)
			fmt.Printf("Tags:         %s\n", t.Tags)
			if t.CloudInit != "" {
				fmt.Printf("CloudInit:    %s\n", t.CloudInit)
			}
			return nil
		},
	}
}

func newTemplateEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			updates := make(map[string]interface{})
			if cmd.Flags().Changed("vm-id") {
				v, _ := cmd.Flags().GetInt("vm-id")
				updates["vm_id"] = v
			}
			if cmd.Flags().Changed("url") {
				v, _ := cmd.Flags().GetString("url")
				updates["url"] = v
			}
			if cmd.Flags().Changed("checksum-url") {
				v, _ := cmd.Flags().GetString("checksum-url")
				updates["checksum_url"] = v
			}
			if cmd.Flags().Changed("tags") {
				v, _ := cmd.Flags().GetString("tags")
				updates["tags"] = v
			}
			if cmd.Flags().Changed("cloudinit") {
				ciName, _ := cmd.Flags().GetString("cloudinit")
				if ciName == "" {
					updates["cloudinit"] = ""
				} else {
					if _, err := db.GetCloudInit(ciName); err != nil {
						return fmt.Errorf("cloud-init config %q not found", ciName)
					}
					updates["cloudinit"] = ciName
				}
			}

			if len(updates) == 0 {
				return fmt.Errorf("no fields to update")
			}

			if err := db.UpdateTemplate(args[0], updates); err != nil {
				return err
			}
			fmt.Printf("Updated template %q\n", args[0])
			return nil
		},
	}
	cmd.Flags().Int("vm-id", 0, "Proxmox VM ID")
	cmd.Flags().String("url", "", "Image download URL")
	cmd.Flags().String("checksum-url", "", "Checksum file URL")
	cmd.Flags().String("tags", "", "Comma-separated tags")
	cmd.Flags().String("cloudinit", "", "Cloud-init config name (empty to unset)")
	return cmd
}

func newTemplateRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := db.DeleteTemplate(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed template %q\n", args[0])
			return nil
		},
	}
}
