package cli

import (
	"context"
	"encoding/json"
	"fmt"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/spf13/cobra"
	"time"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "doctor", Short: "Check node tools, storage, bridge, paths and free space", RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()
		node, err := db.GetNode(cmd.Flag("node").Value.String())
		if err != nil {
			return err
		}
		client, err := managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
		if err != nil {
			return err
		}
		defer client.Close()
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		defer cancel()
		result, err := client.Preflight(ctx, &pb.PreflightRequest{Profile: cmd.Flag("profile").Value.String()})
		if err != nil {
			return err
		}
		failed := false
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if err = json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
				return err
			}
		}
		for _, check := range result.Checks {
			label := "OK"
			if !check.Ok {
				label = "FAIL"
				failed = true
			}
			if !asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "%-4s %s: %s\n", label, check.Name, check.Message)
			}
		}
		if failed {
			return fmt.Errorf("node preflight failed")
		}
		return nil
	}}
	cmd.Flags().String("node", "", "Node to inspect")
	cmd.Flags().String("profile", "", "Node profile")
	cmd.Flags().Bool("json", false, "Print structured checks")
	_ = cmd.MarkFlagRequired("node")
	return cmd
}
