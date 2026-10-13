package engine

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
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
func (e *Engine) BackgroundTasks(id string) []TaskBrief {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]TaskBrief(nil), e.live[id]...)
}

// noteTask keeps the live set current from a recorded "task" entry.
func (e *Engine) noteTask(sessionID string, m Entry) {
	if m.Kind != KindTask {
		return
	}
	var ev TaskEvent
	if json.Unmarshal(m.Payload, &ev) != nil {
		return
	}
	if ev.Event == "task_notification" || ev.Event == "task_completed" {
		e.snapshotOutput(sessionID, m.RequestID, ev)
		return
	}
	if ev.Event != "background_tasks_changed" {
		return
	}
	live := make([]TaskBrief, 0, len(ev.Tasks))
	for _, t := range ev.Tasks {
		live = append(live, TaskBrief{TaskID: t.TaskID, TaskType: t.TaskType, Description: t.Description})
	}
	e.mu.Lock()
	if len(live) == 0 {
		delete(e.live, sessionID)
	} else {
		e.live[sessionID] = live
	}
	e.mu.Unlock()
}

// outputSnapshotSize is how much of a finished command's output is kept.
const outputSnapshotSize = 8 * 1024

// snapshotOutput keeps the end of a finished command's output in the
// transcript: Claude Code writes it to a temporary file that can be gone by
// the time someone looks back (a reboot, its own cleanup), and a finished
// task should stay readable. Subagent output files are their transcripts,
// already recorded entry by entry, so only commands are kept.
func (e *Engine) snapshotOutput(sessionID, callID string, ev TaskEvent) {
	if ev.TaskType != "local_bash" || !ValidOutputPath(ev.OutputFile, ev.TaskID) {
		return
	}
	out, err := ReadTail(ev.OutputFile, outputSnapshotSize)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(TaskEvent{Event: "output_snapshot", TaskID: ev.TaskID, Output: out.Content, Truncated: out.Truncated})
	e.store.Append(sessionID, Entry{Kind: KindTask, RequestID: callID, Payload: payload, Timestamp: time.Now()})
}

// ValidOutputPath accepts only the shape Claude Code uses for a task's
// output file: an absolute …/tasks/<task_id>.output.
func ValidOutputPath(path, taskID string) bool {
	return path != "" && ValidTaskID(taskID) && filepath.IsAbs(path) &&
		filepath.Base(path) == taskID+".output" && filepath.Base(filepath.Dir(path)) == "tasks"
}

// clearTasks forgets id's live tasks once the process that ran them is gone.
func (e *Engine) clearTasks(id string) {
	e.mu.Lock()
	delete(e.live, id)
	e.mu.Unlock()
}

// StopTask stops one of id's background tasks. Claude Code reports the
// outcome through its own events (the task's "stopped" notification).
func (e *Engine) StopTask(ctx context.Context, id, taskID string) error {
	if !e.isLive(id, taskID) {
		return ErrTaskNotRunning
	}
	res, ok := e.residentFor(id)
	if !ok {
		return ErrTaskNotRunning
	}
	return res.c.StopTask(ctx, taskID)
}

func (e *Engine) isLive(id, taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.live[id] {
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

// MaxTaskOutputTail caps how much of a task's output one read returns.
const MaxTaskOutputTail = 256 * 1024

var validTaskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidTaskID reports whether id has the shape of a Claude Code task id.
func ValidTaskID(id string) bool { return validTaskID.MatchString(id) }

// ReadTaskOutput reads the end of a background task's output file: a shell
// command's stdout and exit code, or a subagent's JSONL transcript. path is
// the file Claude Code reported for that task (see TaskOutputFile); only a
// file with the shape it uses (…/tasks/<task_id>.output) is read, else the
// error is os.ErrNotExist. tail is clamped to MaxTaskOutputTail.
func ReadTaskOutput(path, taskID string, tail int) (TaskOutput, error) {
	if !ValidOutputPath(path, taskID) {
		return TaskOutput{}, os.ErrNotExist
	}
	if tail <= 0 || tail > MaxTaskOutputTail {
		tail = MaxTaskOutputTail
	}
	return ReadTail(path, tail)
}

// ReadTail reads the last n bytes of path.
func ReadTail(path string, n int) (TaskOutput, error) {
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

// TaskOutputFile returns the output file a "task" entry reports for taskID,
// if it reports one. A host scans its transcript backwards with it to find
// the file Claude Code last reported.
func TaskOutputFile(kind string, payload json.RawMessage, taskID string) (string, bool) {
	if kind != KindTask {
		return "", false
	}
	var ev TaskEvent
	if json.Unmarshal(payload, &ev) != nil || ev.TaskID != taskID || ev.OutputFile == "" {
		return "", false
	}
	return ev.OutputFile, true
}

// keepAliveInterval is how often a process with background tasks is marked
// active in the pool.
const keepAliveInterval = time.Minute

// keepAlive stops the pool from reclaiming a process whose turn is over but
// whose background tasks are still running: it would end them with it. A
// long command can go minutes without a single event.
func (e *Engine) keepAlive(stop <-chan struct{}) {
	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			e.mu.Lock()
			ids := make([]string, 0, len(e.live))
			for id := range e.live {
				ids = append(ids, id)
			}
			e.mu.Unlock()
			for _, id := range ids {
				e.pool.Touch(id)
			}
		}
	}
}
