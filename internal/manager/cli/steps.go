package cli

import (
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"github.com/spf13/cobra"
)

func newStepsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "steps",
		Short: "Manage build steps",
	}

	cmd.AddCommand(
		newStepsListCmd(),
		newStepsAddCmd(),
		newStepsEditCmd(),
		newStepsRemoveCmd(),
		newStepsResetCmd(),
	)

	return cmd
}

func newStepsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all build steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			steps, err := db.ListBuildSteps()
			if err != nil {
				return err
			}

			if len(steps) == 0 {
				fmt.Println("No build steps found. Run 'pvectgen manager import' or 'pvectgen manager steps reset'.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "#\tNAME\tCOMMAND")
			for i, s := range steps {
				command := s.Command
				if len(command) > 60 {
					command = command[:57] + "..."
				}
				fmt.Fprintf(w, "%d\t%s\t%s\n", i+1, s.Name, command)
			}
			return w.Flush()
		},
	}
}

func newStepsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [name] [command]",
		Short: "Add a build step",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			order, _ := cmd.Flags().GetInt("order")
			if order == 0 {
				// Auto-assign next order
				steps, _ := db.ListBuildSteps()
				order = len(steps) + 1
			}

			if _, err := db.CreateBuildStep(args[0], args[1], order); err != nil {
				return err
			}
			fmt.Printf("Added step %q at position %d\n", args[0], order)
			return nil
		},
	}
	cmd.Flags().Int("order", 0, "Sort order (auto if 0)")
	return cmd
}

func newStepsEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit a build step",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			updates := make(map[string]interface{})
			if cmd.Flags().Changed("command") {
				v, _ := cmd.Flags().GetString("command")
				updates["command"] = v
			}
			if cmd.Flags().Changed("order") {
				v, _ := cmd.Flags().GetInt("order")
				updates["sort_order"] = v
			}

			if len(updates) == 0 {
				return fmt.Errorf("no fields to update")
			}

			if err := db.UpdateBuildStep(args[0], updates); err != nil {
				return err
			}
			fmt.Printf("Updated step %q\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("command", "", "New command")
	cmd.Flags().Int("order", 0, "New sort order")
	return cmd
}

func newStepsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a build step",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := db.DeleteBuildStep(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed step %q\n", args[0])
			return nil
		},
	}
}

func newStepsResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset build steps to defaults (from config/steps.json)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			defaults, err := fileutil.LoadSteps("config/steps.json")
			if err != nil {
				return fmt.Errorf("load default steps: %w", err)
			}

			if err := db.ResetBuildSteps(defaults); err != nil {
				return err
			}
			fmt.Printf("Reset %d build steps to defaults\n", len(defaults))
			return nil
		},
	}
}
