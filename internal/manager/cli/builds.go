package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newBuildsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "builds",
		Short: "View build history",
	}

	cmd.AddCommand(
		newBuildsListCmd(),
		newBuildsShowCmd(),
		newBuildsLogsCmd(),
	)

	return cmd
}

func newBuildsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List builds",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			status, _ := cmd.Flags().GetString("status")
			template, _ := cmd.Flags().GetString("template")
			node, _ := cmd.Flags().GetString("node")
			limit, _ := cmd.Flags().GetInt("limit")

			builds, err := db.ListBuilds(status, template, node, limit)
			if err != nil {
				return err
			}

			if len(builds) == 0 {
				fmt.Println("No builds found.")
				return nil
			}

			w := newTabWriter()
			fmt.Fprintln(w, "BUILD_ID\tSTATUS\tSTARTED\tDURATION")
			for _, b := range builds {
				short := b.BuildID
				if len(short) > 8 {
					short = short[:8]
				}
				started := "-"
				if b.StartedAt != nil {
					started = b.StartedAt.Format("2006-01-02 15:04:05")
				}
				duration := "-"
				if b.StartedAt != nil && b.CompletedAt != nil {
					duration = b.CompletedAt.Sub(*b.StartedAt).Round(1e9).String()
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", short, b.Status, started, duration)
			}
			return w.Flush()
		},
	}
	cmd.Flags().String("status", "", "Filter by status")
	cmd.Flags().String("template", "", "Filter by template")
	cmd.Flags().String("node", "", "Filter by node")
	cmd.Flags().Int("limit", 20, "Max results")
	return cmd
}

func newBuildsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [build-id]",
		Short: "Show build details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			b, err := db.GetBuild(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Build ID: %s\n", b.BuildID)
			fmt.Printf("Status:   %s\n", b.Status)
			if b.StartedAt != nil {
				fmt.Printf("Started:  %s\n", b.StartedAt.Format("2006-01-02 15:04:05"))
			}
			if b.CompletedAt != nil {
				fmt.Printf("Finished: %s\n", b.CompletedAt.Format("2006-01-02 15:04:05"))
			}

			results, err := db.GetBuildStepResults(args[0])
			if err != nil {
				return err
			}

			if len(results) > 0 {
				fmt.Println("\nSteps:")
				for _, r := range results {
					icon := "○"
					switch r.Status {
					case "completed":
						icon = "✓"
					case "failed":
						icon = "✗"
					case "running":
						icon = "◉"
					case "skipped":
						icon = "–"
					}
					fmt.Printf("  %s %s\n", icon, r.StepName)
				}
			}
			return nil
		},
	}
}

func newBuildsLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs [build-id]",
		Short: "Show build logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			results, err := db.GetBuildStepResults(args[0])
			if err != nil {
				return err
			}

			for _, r := range results {
				fmt.Printf("=== %s (%s) ===\n", r.StepName, r.Status)
				if r.Log != "" {
					fmt.Println(r.Log)
				}
				fmt.Println()
			}
			return nil
		},
	}
}
