package desk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tingly-dev/tingly-box/remote/session"
)

// TaskBrief is one background task Claude Code reports as running: a
// backgrounded shell command (local_bash) or subagent (local_agent).
type TaskBrief struct {
	TaskID      string
	TaskType    string
	Description string
}

// BackgroundTasks returns the background tasks of id's live process, as
// Claude Code last reported them (background_tasks_changed). Empty when the
// session has no live process: its tasks ended with it.
func (s *Service) BackgroundTasks(id string) []TaskBrief {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TaskBrief(nil), s.live[id]...)
}

// noteTask keeps the live set current from a recorded "task" entry.
func (s *Service) noteTask(sessionID string, m session.Message) {
	if m.Kind != "task" {
		return
	}
	var ev taskEvent
	if json.Unmarshal(m.Payload, &ev) != nil {
		return
	}
	if ev.Event == "task_notification" || ev.Event == "task_completed" {
		s.snapshotOutput(sessionID, m.RequestID, ev)
		return
	}
	if ev.Event != "background_tasks_changed" {
		return
	}
	live := make([]TaskBrief, 0, len(ev.Tasks))
	for _, t := range ev.Tasks {
		live = append(live, TaskBrief{TaskID: t.TaskID, TaskType: t.TaskType, Description: t.Description})
	}
	s.mu.Lock()
	if len(live) == 0 {
		delete(s.live, sessionID)
	} else {
		s.live[sessionID] = live
	}
	s.mu.Unlock()
}

// outputSnapshotSize is how much of a finished command's output is kept.
const outputSnapshotSize = 8 * 1024

// snapshotOutput keeps the end of a finished command's output in the
// transcript: Claude Code writes it to a temporary file that can be gone by
// the time someone looks back (a reboot, its own cleanup), and a finished
// task should stay readable. Subagent output files are their transcripts,
// already recorded entry by entry, so only commands are kept.
func (s *Service) snapshotOutput(sessionID, callID string, ev taskEvent) {
	if ev.TaskType != "local_bash" || !validOutputPath(ev.OutputFile, ev.TaskID) {
		return
	}
	out, err := readTail(ev.OutputFile, outputSnapshotSize)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(taskEvent{Event: "output_snapshot", TaskID: ev.TaskID, Output: out.Content, Truncated: out.Truncated})
	s.sessions.AppendMessage(sessionID, session.Message{Kind: "task", RequestID: callID, Payload: payload, Timestamp: time.Now()})
}

// validOutputPath accepts only the shape Claude Code uses for a task's
// output file: an absolute …/tasks/<task_id>.output.
func validOutputPath(path, taskID string) bool {
	return path != "" && validTaskID.MatchString(taskID) && filepath.IsAbs(path) &&
		filepath.Base(path) == taskID+".output" && filepath.Base(filepath.Dir(path)) == "tasks"
}

// clearTasks forgets id's live tasks once the process that ran them is gone.
func (s *Service) clearTasks(id string) {
	s.mu.Lock()
	delete(s.live, id)
	s.mu.Unlock()
}

// StopTask stops one of id's background tasks. Claude Code reports the
// outcome through its own events (the task's "stopped" notification).
func (s *Service) StopTask(ctx context.Context, id, taskID string) error {
	if _, ok := s.sessions.SnapshotOrLoad(id); !ok {
		return notFound("session", id)
	}
	if !s.isLive(id, taskID) {
		return conflict("task %s is not running", taskID)
	}
	res, ok := s.residentFor(id)
	if !ok {
		return conflict("task %s is not running", taskID)
	}
	return res.c.StopTask(ctx, taskID)
}

func (s *Service) isLive(id, taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.live[id] {
		if t.TaskID == taskID {
			return true
		}
	}
	return false
}

// TaskOutput is the tail of a background task's output file.
type TaskOutput struct {
	Content   string
	Truncated bool // the file is longer than what Content holds
	Size      int64
}

// maxTaskOutputTail caps how much of a task's output one request returns.
const maxTaskOutputTail = 256 * 1024

var validTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// TaskOutput reads the end of a background task's output file: a shell
// command's stdout and exit code, or a subagent's JSONL transcript. Only a
// file Claude Code itself reported for that task in this session is read,
// and only if it has the shape it uses (…/tasks/<task_id>.output).
func (s *Service) TaskOutput(id, taskID string, tail int) (TaskOutput, error) {
	if !validTaskID.MatchString(taskID) {
		return TaskOutput{}, invalid("bad task id %q", taskID)
	}
	if _, ok := s.sessions.SnapshotOrLoad(id); !ok {
		return TaskOutput{}, notFound("session", id)
	}
	path := s.taskOutputPath(id, taskID)
	if !validOutputPath(path, taskID) {
		return TaskOutput{}, notFound("output of task", taskID)
	}
	if tail <= 0 || tail > maxTaskOutputTail {
		tail = maxTaskOutputTail
	}
	out, err := readTail(path, tail)
	if errors.Is(err, os.ErrNotExist) {
		return TaskOutput{}, notFound("output of task", taskID)
	}
	return out, err
}

// readTail reads the last n bytes of path.
func readTail(path string, n int) (TaskOutput, error) {
	f, err := os.Open(path)
	if err != nil {
		return TaskOutput{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return TaskOutput{}, err
	}
	out := TaskOutput{Size: info.Size()}
	if start := info.Size() - int64(n); start > 0 {
		out.Truncated = true
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return TaskOutput{}, err
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(n)))
	if err != nil {
		return TaskOutput{}, err
	}
	out.Content = string(b)
	return out, nil
}

// taskOutputPath is the output file Claude Code last reported for taskID.
func (s *Service) taskOutputPath(id, taskID string) string {
	msgs, _ := s.sessions.GetMessages(id)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Kind != "task" {
			continue
		}
		var ev taskEvent
		if json.Unmarshal(msgs[i].Payload, &ev) == nil && ev.TaskID == taskID && ev.OutputFile != "" {
			return ev.OutputFile
		}
	}
	return ""
}

// keepAliveInterval is how often a process with background tasks is marked
// active in the pool.
const keepAliveInterval = time.Minute

// keepAlive stops the pool from reclaiming a process whose turn is over but
// whose background tasks are still running: it would end them with it. A
// long command can go minutes without a single event.
func (s *Service) keepAlive(stop <-chan struct{}) {
	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.Lock()
			ids := make([]string, 0, len(s.live))
			for id := range s.live {
				ids = append(ids, id)
			}
			s.mu.Unlock()
			for _, id := range ids {
				s.pool.Touch(id)
			}
		}
	}
}
