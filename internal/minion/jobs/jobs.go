// Package jobs owns build lifetimes and durable event journals independently of RPCs.
package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aloks98/pve-ctgen/internal/shared/filelock"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"google.golang.org/protobuf/proto"
)

type Run func(context.Context, *pb.BuildRequest, chan<- *pb.BuildEvent)
type job struct {
	mu       sync.Mutex
	path     string
	sequence int64
	size     int64
	done     bool
	err      error
	notify   chan struct{}
	cancel   context.CancelFunc
}
type Manager struct {
	mu      sync.Mutex
	dir     string
	jobs    map[string]*job
	run     Run
	closed  bool
	wg      sync.WaitGroup
	release func()
}

func terminal(e *pb.BuildEvent) bool {
	return e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED || e.Type == pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED
}
func New(dir string, run Run) (*Manager, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("jobs directory must be absolute")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Nonblocking exclusive ownership: another server must not recover our active jobs.
	release, err := filelock.Acquire(ctx, filepath.Join(dir, ".owner.lock"))
	if err != nil {
		return nil, err
	}
	m := &Manager{dir: dir, jobs: map[string]*job{}, run: run, release: release}
	entries, err := os.ReadDir(dir)
	if err != nil {
		release()
		return nil, err
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-6]
		j := &job{path: filepath.Join(dir, entry.Name()), notify: make(chan struct{})}
		f, err := os.Open(j.path)
		if err != nil {
			release()
			return nil, err
		}
		reader := bufio.NewReaderSize(f, 8*1024*1024)
		for {
			line, readErr := reader.ReadSlice('\n')
			if readErr == io.EOF {
				// A killed process may leave one partial record; retain every committed line.
				if len(line) > 0 {
					err = os.Truncate(j.path, j.size)
				}
				break
			}
			if readErr != nil {
				err = readErr
				break
			}
			var e pb.BuildEvent
			if err = json.Unmarshal(line, &e); err != nil {
				break
			}
			if e.Sequence != j.sequence+1 || e.BuildId != id || j.done {
				err = fmt.Errorf("invalid journal sequence")
				break
			}
			j.sequence = e.Sequence
			j.done = terminal(&e)
			j.size += int64(len(line))
		}
		f.Close()
		if err != nil {
			release()
			return nil, fmt.Errorf("read journal %s: %w", id, err)
		}

		m.jobs[id] = j
		if !j.done {
			if err = j.append(&pb.BuildEvent{BuildId: id, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: "minion restarted before build finished"}); err != nil {
				release()
				return nil, err
			}
		}
	}
	return m, nil
}
func (j *job) append(e *pb.BuildEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return fmt.Errorf("job already finished")
	}
	e = proto.Clone(e).(*pb.BuildEvent)
	e.Sequence = j.sequence + 1
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Reserve space for a terminal record when noisy commands hit the journal limit.
	if j.size+int64(len(data)) > 64*1024*1024 && !terminal(e) {
		return fmt.Errorf("build journal exceeds 64 MiB")
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		j.err = err
		close(j.notify)
		j.notify = make(chan struct{})
		return err
	}
	j.sequence = e.Sequence
	j.size += int64(len(data))
	j.done = terminal(e)
	close(j.notify)
	j.notify = make(chan struct{})
	return nil
}
func (m *Manager) Start(req *pb.BuildRequest) error {
	if err := validation.Build(req); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("minion shutting down")
	}
	if _, ok := m.jobs[req.BuildId]; ok {
		return fmt.Errorf("build ID already exists; use watch to reconnect")
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{path: filepath.Join(m.dir, req.BuildId+".jsonl"), notify: make(chan struct{}), cancel: cancel}
	if err := j.append(&pb.BuildEvent{BuildId: req.BuildId, Type: pb.BuildEventType_BUILD_EVENT_TYPE_LOG, Message: "Build accepted"}); err != nil {
		cancel()
		return err
	}
	m.jobs[req.BuildId] = j
	m.wg.Add(1)
	req = proto.Clone(req).(*pb.BuildRequest)
	go func() {
		defer m.wg.Done()
		defer cancel()
		events := make(chan *pb.BuildEvent, 100)
		go m.run(ctx, req, events)
		var failure error
		for e := range events {
			e = proto.Clone(e).(*pb.BuildEvent)
			e.BuildId = req.BuildId
			if terminal(e) && ctx.Err() != nil {
				e.Type = pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED
				e.Message = "build cancelled"
			}
			if failure == nil {
				failure = j.append(e)
				if failure != nil {
					cancel()
				}
			}
		}
		j.mu.Lock()
		done := j.done
		j.mu.Unlock()
		if !done {
			msg := "build ended without a terminal event"
			if ctx.Err() != nil {
				msg = "build cancelled"
			}
			if failure != nil {
				msg = failure.Error()
			}
			_ = j.append(&pb.BuildEvent{BuildId: req.BuildId, Type: pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED, Message: msg})
		}
	}()
	return nil
}
func (m *Manager) get(id string) (*job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil {
		return nil, fmt.Errorf("build %q not found", id)
	}
	return j, nil
}
func (m *Manager) Cancel(id string) (bool, error) {
	j, err := m.get(id)
	if err != nil {
		return false, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return false, nil
	}
	if j.cancel != nil {
		j.cancel()
	}
	return true, nil
}
func (m *Manager) Watch(ctx context.Context, id string, after int64, send func(*pb.BuildEvent) error) error {
	if after < 0 {
		return fmt.Errorf("after_sequence must be nonnegative")
	}
	j, err := m.get(id)
	if err != nil {
		return err
	}
	f, err := os.Open(j.path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var cursor int64
	for {
		j.mu.Lock()
		notify, done, writeErr, last := j.notify, j.done, j.err, j.sequence
		j.mu.Unlock()
		for cursor < last {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				return err
			}
			var e pb.BuildEvent
			if err = json.Unmarshal(line, &e); err != nil {
				return err
			}
			cursor = e.Sequence
			if e.Sequence > after {
				if err = send(&e); err != nil {
					return err
				}
			}
		}
		if writeErr != nil {
			return writeErr
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, j := range m.jobs {
		j.mu.Lock()
		if j.cancel != nil {
			j.cancel()
		}
		j.mu.Unlock()
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.release()
}
