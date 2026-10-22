package store

import (
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/models"
	"github.com/aloks98/pve-ctgen/internal/shared/validation"
	pb "github.com/aloks98/pve-ctgen/proto/pvectgen/v1"
	"time"
)

// ApplyBuildEvent atomically persists an event and its replay cursor.
func (db *DB) ApplyBuildEvent(e *pb.BuildEvent) error {
	if err := validation.Name(e.BuildId); err != nil {
		return err
	}
	unlock, err := db.lock()
	if err != nil {
		return err
	}
	defer unlock()
	var b models.Build
	if err = readYAML(db.buildPath(e.BuildId), &b); err != nil {
		return err
	}
	if b.BuildID == "" {
		return fmt.Errorf("build not found")
	}
	if e.Sequence > 0 && e.Sequence <= b.LastSequence {
		return nil
	}
	now := time.Now()
	var step *models.BuildStepResult
	if e.StepName != "" {
		for i := range b.Steps {
			if b.Steps[i].StepName == e.StepName {
				step = &b.Steps[i]
				break
			}
		}
		if step == nil {
			b.Steps = append(b.Steps, models.BuildStepResult{StepName: e.StepName, StepIndex: int(e.StepIndex), Status: "running", StartedAt: &now})
			step = &b.Steps[len(b.Steps)-1]
		}
	}
	switch e.Type {
	case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_COMPLETED:
		if step != nil {
			step.Status = "completed"
			step.CompletedAt = &now
		}
	case pb.BuildEventType_BUILD_EVENT_TYPE_STEP_FAILED:
		if step != nil {
			step.Status = "failed"
			step.CompletedAt = &now
		}
	case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_COMPLETED:
		b.Status = "completed"
		b.CompletedAt = &now
	case pb.BuildEventType_BUILD_EVENT_TYPE_BUILD_FAILED:
		b.Status = "failed"
		if e.Message == "build cancelled" {
			b.Status = "cancelled"
		}
		b.CompletedAt = &now
	}
	if e.Message != "" {
		if step != nil {
			step.Log += e.Message + "\n"
		} else {
			b.Log += e.Message + "\n"
		}
	}
	if e.Sequence > 0 {
		b.LastSequence = e.Sequence
	}
	return writeYAML(db.buildPath(b.BuildID), b)
}
