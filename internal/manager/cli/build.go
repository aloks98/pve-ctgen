package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"

	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
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

			// Get node
			if nodeName == "" {
				return fmt.Errorf("--node is required")
			}
			node, err := db.GetNode(nodeName)
			if err != nil {
				return fmt.Errorf("node %q: %w", nodeName, err)
			}

			// Get templates
			var templates []struct {
				vmID int
				name string
				url  string
				csURL string
				tags string
				ciContent []byte
				ciFilename string
			}

			if all {
				allTemplates, err := db.ListTemplates()
				if err != nil {
					return err
				}
				for _, t := range allTemplates {
					entry := struct {
						vmID int
						name string
						url  string
						csURL string
						tags string
						ciContent []byte
						ciFilename string
					}{t.VMID, t.Name, t.URL, t.ChecksumURL, t.Tags, nil, ""}
					if t.CloudInit != "" {
						ci, err := db.GetCloudInitByID(t.CloudInit)
						if err == nil {
							entry.ciContent = []byte(ci.Content)
							entry.ciFilename = ci.Name
						}
					}
					templates = append(templates, entry)
				}
			} else {
				for _, name := range templateNames {
					t, err := db.GetTemplate(name)
					if err != nil {
						return fmt.Errorf("template %q: %w", name, err)
					}
					entry := struct {
						vmID int
						name string
						url  string
						csURL string
						tags string
						ciContent []byte
						ciFilename string
					}{t.VMID, t.Name, t.URL, t.ChecksumURL, t.Tags, nil, ""}
					if t.CloudInit != "" {
						ci, err := db.GetCloudInitByID(t.CloudInit)
						if err == nil {
							entry.ciContent = []byte(ci.Content)
							entry.ciFilename = ci.Name
						}
					}
					templates = append(templates, entry)
				}
			}

			if len(templates) == 0 {
				return fmt.Errorf("no templates selected")
			}

			// Get build steps
			steps, err := db.ListBuildSteps()
			if err != nil {
				return err
			}
			var pbSteps []*pb.BuildStep
			for _, s := range steps {
				pbSteps = append(pbSteps, &pb.BuildStep{Name: s.Name, Command: s.Command})
			}

			// Connect to node
			client, err := managergrpc.NewClient(node.Address, node.APIKey)
			if err != nil {
				return fmt.Errorf("connect to %s: %w", node.Name, err)
			}
			defer client.Close()

			// Run builds
			for _, t := range templates {
				buildID := uuid.New().String()

				if _, err := db.CreateBuild(buildID, t.name, node.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to record build for %s: %v\n", t.name, err)
					continue
				}

				fmt.Printf("Building %s (build: %s)...\n", t.name, buildID[:8])

				req := &pb.BuildRequest{
					BuildId: buildID,
					Image: &pb.Image{
						Id:          int32(t.vmID),
						Name:        t.name,
						Url:         t.url,
						ChecksumUrl: t.csURL,
						Tags:        t.tags,
					},
					Steps:             pbSteps,
					CloudinitContent:  t.ciContent,
					CloudinitFilename: t.ciFilename,
				}

				stream, err := client.Build(context.Background(), req)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Build RPC failed for %s: %v\n", t.name, err)
					db.UpdateBuildStatus(buildID, "failed")
					continue
				}

				var buildFailed bool
				for {
					event, err := stream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						fmt.Fprintf(os.Stderr, "Stream error: %v\n", err)
						buildFailed = true
						break
					}

					switch event.Type {
					case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_STARTED:
						db.CreateBuildStepResult(buildID, event.StepName, int(event.StepIndex))
						fmt.Printf("  [%s] started\n", event.StepName)
					case pb.BuildEventType_BUILD_EVENT_TYPE_LOG:
						db.AppendBuildStepLog(buildID, event.StepName, event.Message)
					case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED:
						db.UpdateBuildStepResult(buildID, event.StepName, "completed", "")
						fmt.Printf("  [%s] completed\n", event.StepName)
					case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED:
						db.UpdateBuildStepResult(buildID, event.StepName, "failed", event.Message)
						fmt.Printf("  [%s] FAILED: %s\n", event.StepName, event.Message)
					case pb.BuildEventType_BUILD_EVENT_TYPE_DOWNLOAD_PROGRESS:
						fmt.Printf("\r  Downloading: %.0f%%", event.Progress*100)
					case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED:
						fmt.Printf("\n  Build completed!\n")
					case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
						fmt.Printf("\n  Build FAILED: %s\n", event.Message)
						buildFailed = true
					}
				}

				if buildFailed {
					db.UpdateBuildStatus(buildID, "failed")
				} else {
					db.UpdateBuildStatus(buildID, "completed")
				}
			}

			return nil
		},
	}
	cmd.Flags().StringSlice("template", nil, "Template names to build")
	cmd.Flags().String("node", "", "Target node name")
	cmd.Flags().Bool("all", false, "Build all templates")
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

			if err := db.UpdateBuildStatus(args[0], "cancelled"); err != nil {
				return err
			}
			fmt.Printf("Cancelled build %s\n", args[0])
			return nil
		},
	}
}
