// Package buildrun is the shared CLI/TUI build coordinator.
package buildrun

import (
	"context"
	"errors"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/manager/initresolve"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
)

type Client interface {
	Build(context.Context, *pb.BuildRequest) (pb.MinionService_BuildClient, error)
}
type Stream interface {
	Recv() (*pb.BuildEvent, error)
}

func Run(ctx context.Context, db *store.DB, client Client, tmpl models.Template, node string, options map[string]string, emit func(*pb.BuildEvent)) error {
	req, err := Prepare(db, tmpl, options)
	if err != nil {
		return err
	}
	return Start(ctx, db, client, req, node, emit)
}

// Prepare resolves and validates a snapshot without creating a job or history entry.
func Prepare(db *store.DB, tmpl models.Template, options map[string]string) (*pb.BuildRequest, error) {
	copyOptions := make(map[string]string, len(options))
	for k, v := range options {
		copyOptions[k] = v
	}
	options = copyOptions
	if tmpl.VMID < 100 || tmpl.VMID > 999999999 {
		return nil, fmt.Errorf("VM ID must be between 100 and 999999999")
	}
	if options["profile"] == "" && tmpl.Profile != "" {
		options["profile"] = tmpl.Profile
	}
	content, filename, err := initresolve.Resolve(db, tmpl)
	if err != nil {
		return nil, err
	}
	steps, err := db.ListBuildSteps()
	if err != nil {
		return nil, err
	}
	req := &pb.BuildRequest{BuildId: uuid.NewString(), Image: &pb.Image{Id: int32(tmpl.VMID), Name: tmpl.Name, Url: tmpl.URL, ChecksumUrl: tmpl.ChecksumURL, Tags: tmpl.Tags, Vendor: filename}, CloudinitContent: content, CloudinitFilename: filename, InitType: tmpl.EffectiveInitType(), Options: options}
	for _, step := range steps {
		req.Steps = append(req.Steps, &pb.BuildStep{Name: step.Name, Command: step.Command})
	}
	if err = validation.Build(req); err != nil {
		return nil, err
	}
	return req, nil
}

// Start submits the exact request reviewed by the caller.
func Start(ctx context.Context, db *store.DB, client Client, req *pb.BuildRequest, node string, emit func(*pb.BuildEvent)) error {
	if err := validation.Build(req); err != nil {
		return err
	}

	if _, err := db.CreateBuild(req.BuildId, req.Image.Name, node); err != nil {
		return err
	}
	emit(&pb.BuildEvent{BuildId: req.BuildId, Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: fmt.Sprintf("Building %s (%s)", req.Image.Name, req.BuildId)})
	stream, err := client.Build(ctx, req)
	if err != nil {
		return errors.Join(err, db.UpdateBuildStatus(req.BuildId, "disconnected"))
	}
	return Consume(db, req.BuildId, stream, emit)
}
func Consume(db *store.DB, id string, stream Stream, emit func(*pb.BuildEvent)) error {
	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				err = fmt.Errorf("build stream ended without a final status")
			}
			return errors.Join(fmt.Errorf("build %s: %w; reconnect with build watch", id, err), db.UpdateBuildStatus(id, streamFailureStatus(err)))
		}
		if event.BuildId != id {
			return fmt.Errorf("unexpected build ID in stream")
		}
		if err = db.ApplyBuildEvent(event); err != nil {
			return fmt.Errorf("persist build %s: %w", id, err)
		}
		emit(event)
		switch event.Type {
		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED:
			return nil
		case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
			return fmt.Errorf("build %s failed: %s", id, event.Message)
		}
	}
}

func streamFailureStatus(err error) string {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.Unauthenticated, codes.PermissionDenied, codes.FailedPrecondition, codes.NotFound:
		return "failed"
	}
	return "disconnected"
}
