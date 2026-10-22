package buildrun

import (
	"github.com/aloks98/pve-ctgen/internal/manager/store"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"io"
	"testing"
)

type stream struct{ events []*pb.BuildEvent }

func (s *stream) Recv() (*pb.BuildEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}
func TestTerminalRequiredAndReplayIdempotent(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.CreateBuild("job", "test", "node")
	log := &pb.BuildEvent{BuildId: "job", Sequence: 1, Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, StepName: "run", Message: "one line"}
	emit := func(*pb.BuildEvent) {}
	if err = Consume(db, "job", &stream{[]*pb.BuildEvent{log}}, emit); err == nil {
		t.Fatal("early EOF accepted")
	}
	b, _ := db.GetBuild("job")
	if b.Status != "disconnected" {
		t.Fatal(b.Status)
	}
	done := &pb.BuildEvent{BuildId: "job", Sequence: 2, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED}
	if err = Consume(db, "job", &stream{[]*pb.BuildEvent{log, done}}, emit); err != nil {
		t.Fatal(err)
	}
	b, _ = db.GetBuild("job")
	if b.Status != "completed" || b.Steps[0].Log != "one line\n" || b.LastSequence != 2 {
		t.Fatalf("%+v", b)
	}
}
