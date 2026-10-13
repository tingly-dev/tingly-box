package desk

import (
	"context"
	"errors"
	"os"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
)

// TaskBrief is one background task Claude Code reports as running.
type TaskBrief = engine.TaskBrief

// TaskOutput is the tail of a background task's output file.
type TaskOutput = engine.TaskOutput

// BackgroundTasks returns the background tasks of id's live process.
func (s *Service) BackgroundTasks(id string) []TaskBrief { return s.engine.BackgroundTasks(id) }

// StopTask stops one of id's background tasks.
func (s *Service) StopTask(ctx context.Context, id, taskID string) error {
	if _, ok := s.sessions.SnapshotOrLoad(id); !ok {
		return notFound("session", id)
	}
	if err := s.engine.StopTask(ctx, id, taskID); errors.Is(err, engine.ErrTaskNotRunning) {
		return conflict("task %s is not running", taskID)
	} else if err != nil {
		return err
	}
	return nil
}

// TaskOutput reads the end of a background task's output file. Only the
// file Claude Code itself reported for that task in this session is read.
func (s *Service) TaskOutput(id, taskID string, tail int) (TaskOutput, error) {
	if !engine.ValidTaskID(taskID) {
		return TaskOutput{}, invalid("bad task id %q", taskID)
	}
	if _, ok := s.sessions.SnapshotOrLoad(id); !ok {
		return TaskOutput{}, notFound("session", id)
	}
	out, err := engine.ReadTaskOutput(s.taskOutputPath(id, taskID), taskID, tail)
	if errors.Is(err, os.ErrNotExist) {
		return TaskOutput{}, notFound("output of task", taskID)
	}
	return out, err
}

// taskOutputPath is the output file Claude Code last reported for taskID.
func (s *Service) taskOutputPath(id, taskID string) string {
	msgs, _ := s.sessions.GetMessages(id)
	for i := len(msgs) - 1; i >= 0; i-- {
		if path, ok := engine.TaskOutputFile(msgs[i].Kind, msgs[i].Payload, taskID); ok {
			return path
		}
	}
	return ""
}
