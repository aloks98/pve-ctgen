package filelock

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestContentionCancellationAndRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.lock")
	release, err := Acquire(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, p); err == nil {
		t.Fatal("second holder acquired lock")
	}
	release()
	release()
	release, err = Acquire(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
