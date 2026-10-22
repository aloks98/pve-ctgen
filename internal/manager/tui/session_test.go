package tui

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/aloks98/pve-ctgen/internal/manager/store"
	"github.com/aloks98/pve-ctgen/internal/manager/tui/views"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"google.golang.org/grpc"
)

type sessionStream struct {
	grpc.ClientStream
	events []*pb.BuildEvent
}

func (s *sessionStream) Recv() (*pb.BuildEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}

type fakeSessionClient struct {
	builds, watches int
	after           int64
}

func (c *fakeSessionClient) Close() error { return nil }
func (c *fakeSessionClient) Build(_ context.Context, r *pb.BuildRequest) (pb.MinionService_BuildClient, error) {
	c.builds++
	return &sessionStream{events: []*pb.BuildEvent{{BuildId: r.BuildId, Sequence: 1, Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: "before disconnect"}}}, nil
}
func (c *fakeSessionClient) WatchBuild(_ context.Context, r *pb.WatchBuildRequest) (pb.MinionService_WatchBuildClient, error) {
	c.watches++
	c.after = r.AfterSequence
	return &sessionStream{events: []*pb.BuildEvent{{BuildId: r.BuildId, Sequence: 2, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED}}}, nil
}
func (c *fakeSessionClient) CancelBuild(context.Context, string) (*pb.CancelBuildResponse, error) {
	return &pb.CancelBuildResponse{Cancelled: true}, nil
}

func TestSessionReconnectUsesPersistedCursorWithoutResubmission(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := newBuildSession(models.Node{Name: "tokyo"})
	defer s.close()
	client := &fakeSessionClient{}
	s.connect = func() (sessionClient, error) { return client, nil }
	req := &pb.BuildRequest{BuildId: "test-build", Image: &pb.Image{Id: 9400, Name: "debian", Url: "https://example.com/image", ChecksumUrl: "https://example.com/SHA256SUMS"}, Steps: []*pb.BuildStep{{Name: "Configure", Command: "echo ok"}}}
	go s.run(db, []*pb.BuildRequest{req})
	disconnected := false
	done := false
	deadline := time.After(3 * time.Second)
	for !done {
		select {
		case msg, ok := <-s.events:
			if !ok {
				done = true
				break
			}
			if _, ok := msg.(views.BuildConnectionMsg); ok {
				disconnected = true
				s.reconnect()
			}
		case <-deadline:
			t.Fatal("session did not finish")
		}
	}
	if !disconnected || client.builds != 1 || client.watches != 1 || client.after != 1 {
		t.Fatalf("reconnect: %+v disconnected=%v", client, disconnected)
	}
	record, err := db.GetBuild("test-build")
	if err != nil || record.Status != "completed" || record.LastSequence != 2 {
		t.Fatalf("record: %+v %v", record, err)
	}
}

func TestSessionCancelStopsQueuedBuilds(t *testing.T) {
	s := newBuildSession(models.Node{Name: "tokyo"})
	defer s.close()
	client := &fakeSessionClient{}
	s.connect = func() (sessionClient, error) { return client, nil }
	result := s.cancelBuild("test-build")().(views.BuildCancelResultMsg)
	if result.Err != nil || !s.stopQueue.Load() {
		t.Fatal("cancel did not stop queue")
	}
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.run(db, []*pb.BuildRequest{{BuildId: "queued"}})
	if client.builds != 0 {
		t.Fatal("started queued build after cancellation")
	}
}
