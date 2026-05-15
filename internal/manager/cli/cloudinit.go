package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/aloks98/pve-ctgen/internal/shared/cloudinit"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/spf13/cobra"
)

func newCloudInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloudinit",
		Short: "Manage cloud-init configurations",
	}

	cmd.AddCommand(
		newCloudInitListCmd(),
		newCloudInitAddCmd(),
		newCloudInitShowCmd(),
		newCloudInitEditCmd(),
		newCloudInitRemoveCmd(),
	)

	return cmd
}

func newCloudInitListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all cloud-init configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			configs, err := db.ListCloudInits()
			if err != nil {
				return err
			}

			if len(configs) == 0 {
				fmt.Println("No cloud-init configs found. Run 'pvectgen manager import' to import from files.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "NAME\tTYPE\tUPDATED")
			for _, c := range configs {
				fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, c.Type, c.UpdatedAt.Format("2006-01-02"))
			}
			return w.Flush()
		},
	}
}

func newCloudInitAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [name] [file]",
		Short: "Add a cloud-init config from a YAML file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			content, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			warnings, err := cloudinit.ValidateStrict(string(content))
			if err != nil {
				return fmt.Errorf("invalid cloud-init YAML: %w", err)
			}
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}

			if _, err := db.CreateCloudInit(args[0], string(content)); err != nil {
				return err
			}
			fmt.Printf("Added cloud-init config %q\n", args[0])
			return nil
		},
	}
}

func newCloudInitShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show a cloud-init config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			c, err := db.GetCloudInit(args[0])
			if err != nil {
				return err
			}
			fmt.Println(c.Content)
			return nil
		},
	}
}

func newCloudInitEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a cloud-init config in $EDITOR (default: nvim)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			c, err := db.GetCloudInit(args[0])
			if err != nil {
				return err
			}

			tmpFile, err := os.CreateTemp("", "pvectgen-cloudinit-*.yaml")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(c.Content); err != nil {
				return err
			}
			tmpFile.Close()

			editor := fileutil.Editor()
			editorCmd := exec.Command(editor, tmpFile.Name())
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			if err := editorCmd.Run(); err != nil {
				return fmt.Errorf("editor failed: %w", err)
			}

			content, err := os.ReadFile(tmpFile.Name())
			if err != nil {
				return err
			}

			warnings, verr := cloudinit.ValidateStrict(string(content))
			if verr != nil {
				return fmt.Errorf("invalid cloud-init YAML: %w", verr)
			}
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}

			if err := db.UpdateCloudInit(args[0], string(content)); err != nil {
				return err
			}
			fmt.Printf("Updated cloud-init config %q\n", args[0])
			return nil
		},
	}
}

func newCloudInitRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a cloud-init config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := db.DeleteCloudInit(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed cloud-init config %q\n", args[0])
			return nil
		},
	}
}
