package tui

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/aloks98/pve-ctgen/internal/manager/buildrun"
	managergrpc "github.com/aloks98/pve-ctgen/internal/manager/grpc"
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/views"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A session owns a sequential queue and its stream. Reconnect only watches an
// existing job; it never resubmits a Build RPC with an ambiguous outcome.
type buildSession struct {
	connect   func() (sessionClient, error)
	events    chan tea.Msg
	retry     chan struct{}
	node      models.Node
	stopQueue atomic.Bool
	ctx       context.Context
	close     context.CancelFunc
}

type sessionClient interface {
	buildrun.Client
	WatchBuild(context.Context, *pb.WatchBuildRequest) (pb.MinionService_WatchBuildClient, error)
	CancelBuild(context.Context, string) (*pb.CancelBuildResponse, error)
	Close() error
}

func newBuildSession(node models.Node) *buildSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &buildSession{events: make(chan tea.Msg, 100), retry: make(chan struct{}, 1), node: node, ctx: ctx, close: cancel, connect: func() (sessionClient, error) {
		return managergrpc.NewClient(node.Address, node.APIKey, node.TLSFingerprint)
	}}
}
func (s *buildSession) send(msg tea.Msg) bool {
	select {
	case s.events <- msg:
		return true
	case <-s.ctx.Done():
		return false
	}
}
func (s *buildSession) reconnect() {
	select {
	case s.retry <- struct{}{}:
	default:
	}
}
func (s *buildSession) wait(err error) bool {
	if !s.send(views.BuildConnectionMsg{Err: err}) {
		return false
	}
	select {
	case <-s.ctx.Done():
		return false
	case <-s.retry:
		return true
	}
}
func (s *buildSession) run(db *store.DB, requests []*pb.BuildRequest) {
	defer close(s.events)
	for _, req := range requests {
		if s.stopQueue.Load() || s.ctx.Err() != nil {
			return
		}
		s.send(views.BuildStartedMsg{Request: req, Node: s.node.Name})
		submitted := false
		for s.ctx.Err() == nil {
			client, err := s.connect()
			if err != nil {
				if !s.wait(err) {
					return
				}
				continue
			}
			s.send(views.BuildConnectedMsg{})
			terminal := false
			emit := func(e *pb.BuildEvent) {
				if e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED || e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED {
					terminal = true
				}
				s.send(views.BuildEventMsg{Event: e})
			}
			if !submitted {
				if s.stopQueue.Load() {
					client.Close()
					return
				}
				submitted = true
				err = buildrun.Start(s.ctx, db, client, req, s.node.Name, emit)
			} else {
				record, readErr := db.GetBuild(req.BuildId)
				if readErr != nil {
					client.Close()
					s.send(views.BuildEventMsg{Event: &pb.BuildEvent{BuildId: req.BuildId, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: readErr.Error()}})
					break
				}
				after := record.LastSequence
				// A durable terminal event may have been saved before its UI delivery.
				// Include it again so an empty replay cannot look disconnected forever.
				if record.Status == "completed" || record.Status == "failed" {
					after = max(0, after-1)
				}
				stream, watchErr := client.WatchBuild(s.ctx, &pb.WatchBuildRequest{BuildId: req.BuildId, AfterSequence: after})
				err = watchErr
				if err == nil {
					err = buildrun.Consume(db, req.BuildId, stream, emit)
				}
			}
			client.Close()
			if terminal {
				break
			}
			if err == nil {
				err = fmt.Errorf("stream ended without a final build status")
			}
			record, readErr := db.GetBuild(req.BuildId)
			if readErr != nil || record.Status == "failed" || status.Code(err) == codes.InvalidArgument || status.Code(err) == codes.FailedPrecondition || status.Code(err) == codes.NotFound || status.Code(err) == codes.PermissionDenied || status.Code(err) == codes.Unauthenticated {
				if readErr == nil {
					_ = db.UpdateBuildStatus(req.BuildId, "failed")
				}
				s.send(views.BuildEventMsg{Event: &pb.BuildEvent{BuildId: req.BuildId, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: err.Error()}})
				break
			}
			if !s.wait(err) {
				return
			}
		}
	}
}
func (s *buildSession) cancelBuild(id string) tea.Cmd {
	s.stopQueue.Store(true)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
		defer cancel()
		client, err := s.connect()
		if err != nil {
			return views.BuildCancelResultMsg{ID: id, Err: err}
		}
		defer client.Close()
		result, err := client.CancelBuild(ctx, id)
		if err == nil && !result.Cancelled {
			err = fmt.Errorf("%s", result.Message)
		}
		if err == nil {
			s.reconnect()
		}
		return views.BuildCancelResultMsg{ID: id, Err: err}
	}
}
