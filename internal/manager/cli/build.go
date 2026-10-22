package cli

import (
	"errors"
	"fmt"

	"github.com/aloks98/pve-ctgen/internal/manager/buildrun"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build operations",
	}

	cmd.AddCommand(
		newBuildRunCmd(),
		newBuildCancelCmd(),
		newBuildWatchCmd(),
	)

	return cmd
}

func newBuildRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Trigger a build on a Minion node",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			templateNames, _ := cmd.Flags().GetStringSlice("template")
			nodeName, _ := cmd.Flags().GetString("node")
			all, _ := cmd.Flags().GetBool("all")

			if nodeName == "" {
				return fmt.Errorf("--node is required")
			}
			node, err := db.GetNode(nodeName)
			if err != nil {
				return fmt.Errorf("node %q: %w", nodeName, err)
			}

			// Collect templates
			var templates []models.Template
			if all {
				templates, err = db.ListTemplates()
				if err != nil {
					return err
				}
			} else {
				for _, name := range templateNames {
					t, err := db.GetTemplate(name)
					if err != nil {
						return fmt.Errorf("template %q: %w", name, err)
					}
					templates = append(templates, *t)
				}
			}
			if len(templates) == 0 {
				return fmt.Errorf("no templates selected")
			}

			client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
			if err != nil {
				return err
			}
			defer client.Close()
			var failures []error
			for _, tmpl := range templates {
				err := buildrun.Run(cmd.Context(), db, client, tmpl, node.Name, map[string]string{"overwrite": cmd.Flag("overwrite").Value.String(), "profile": cmd.Flag("profile").Value.String()}, printBuildEvent)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", tmpl.Name, err))
				}
			}
			return errors.Join(failures...)

		},
	}
	cmd.Flags().StringSlice("template", nil, "Template names to build")
	cmd.Flags().String("node", "", "Target node name")
	cmd.Flags().Bool("all", false, "Build all templates")
	cmd.Flags().String("profile", "", "Override the template/node build profile")
	cmd.Flags().Bool("overwrite", false, "Replace an existing template (never a regular VM)")
	return cmd
}

func newBuildCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel [build-id]",
		Short: "Cancel a running build",
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
			node, err := db.GetNode(b.Node)
			if err != nil {
				return err
			}
			client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
			if err != nil {
				return err
			}
			defer client.Close()
			resp, err := client.CancelBuild(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Println(resp.Message)
			return nil

		},
	}
}

func printBuildEvent(e *pb.BuildEvent) {
	message := e.Message
	switch e.Type {
	case pb.BuildEventType_BUILD_EVENT_TYPE_DOWNLOAD_PROGRESS:
		return
	case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED:
		message = "started"
	case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED:
		message = "completed"
	case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED:
		message = "FAILED: " + message
	case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
		message = "BUILD FAILED: " + message
	}
	if e.StepName != "" {
		fmt.Printf("  [%s] %s\n", e.StepName, message)
	} else {
		fmt.Println(message)
	}
}
func newBuildWatchCmd() *cobra.Command {
	return &cobra.Command{Use: "watch [build-id]", Short: "Reconnect and persist remaining build events", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()
		b, err := db.GetBuild(args[0])
		if err != nil {
			return err
		}
		node, err := db.GetNode(b.Node)
		if err != nil {
			return err
		}
		client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
		if err != nil {
			return err
		}
		defer client.Close()
		// Replay all events; the store cursor makes persistence idempotent and includes terminal status.
		stream, err := client.WatchBuild(cmd.Context(), &pb.WatchBuildRequest{BuildId: b.BuildID})
		if err != nil {
			return err
		}
		return buildrun.Consume(db, b.BuildID, stream, printBuildEvent)
	}}
}
