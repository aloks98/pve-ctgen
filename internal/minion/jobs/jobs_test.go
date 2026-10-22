package jobs

import (
	"context"
	"errors"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func request(id string) *pb.BuildRequest {
	return &pb.BuildRequest{BuildId: id, Image: &pb.Image{Id: 100, Name: "test", Url: "https://example.com/disk", ChecksumUrl: "https://example.com/sums"}, Steps: []*pb.BuildStep{{Name: "test", Command: "true"}}}
}
func TestDisconnectReplayCancel(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, func(ctx context.Context, r *pb.BuildRequest, out chan<- *pb.BuildEvent) {
		defer close(out)
		out <- &pb.BuildEvent{Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: "running"}
		<-ctx.Done()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Start(request("job")); err != nil {
		t.Fatal(err)
	}
	if err = m.Start(request("job")); err == nil {
		t.Fatal("duplicate accepted")
	}
	detached := errors.New("client left")
	if err = m.Watch(context.Background(), "job", 0, func(*pb.BuildEvent) error { return detached }); !errors.Is(err, detached) {
		t.Fatal(err)
	}
	if ok, err := m.Cancel("job"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var events []*pb.BuildEvent
	if err = m.Watch(ctx, "job", 0, func(e *pb.BuildEvent) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || !terminal(events[len(events)-1]) {
		t.Fatal(events)
	}
	m.Close()
	m, err = New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	var replay []*pb.BuildEvent
	if err = m.Watch(ctx, "job", events[len(events)-2].Sequence, func(e *pb.BuildEvent) error { replay = append(replay, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].Message != "build cancelled" {
		t.Fatal(replay)
	}
}
func TestRestartMarksIncompleteFailed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "job.jsonl"), []byte("{\"build_id\":\"job\",\"sequence\":1,\"type\":2}\n{\"partial"), 0600)
	m, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	var last *pb.BuildEvent
	if err = m.Watch(context.Background(), "job", 0, func(e *pb.BuildEvent) error { last = e; return nil }); err != nil {
		t.Fatal(err)
	}
	if last.Type != pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED {
		t.Fatal(last)
	}
}
