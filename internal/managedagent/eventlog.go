package managedagent

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

	"github.com/tingly-dev/tingly-box/pkg/fs"
)

// EventLog is the file-backed EventStore: one append-only JSONL file per
// session, the same shape as remote/session.Transcript and for the same
// reason — a session's log is write-once, read in order, never queried by
// content, and unbounded, so it does not belong in the shared SQLite file.
//
// Seq is assigned here, per session, from the last line of the file on first
// touch and then in memory; a single mutex serialises appends so a line is
// never torn and two appends never share a Seq.
type EventLog struct {
	dir  string
	mu   sync.Mutex
	last map[string]int64
}

var _ EventStore = (*EventLog)(nil)

// NewEventLog opens (creating) the log directory.
func NewEventLog(dir string) (*EventLog, error) {
	if dir == "" {
		return nil, errors.New("event log dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create event log dir: %w", err)
	}
	return &EventLog{dir: dir, last: map[string]int64{}}, nil
}

// Path returns the on-disk file for a session's log.
func (l *EventLog) Path(sessionID string) string {
	return filepath.Join(l.dir, fs.SafeFileKey(sessionID)+".jsonl")
}

// AppendEvent assigns the next Seq and writes one line.
func (l *EventLog) AppendEvent(_ context.Context, e *Event) error {
	if e == nil || e.SessionID == "" {
		return invalid("event needs a session id")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	last, ok := l.last[e.SessionID]
	if !ok {
		var err error
		last, err = l.lastSeq(e.SessionID)
		if err != nil {
			return err
		}
	}
	e.Seq = last + 1

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	f, err := os.OpenFile(l.Path(e.SessionID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	l.last[e.SessionID] = e.Seq
	return nil
}

// ListEvents reads the log and returns events with Seq > after.
func (l *EventLog) ListEvents(_ context.Context, sessionID string, after int64, limit int) ([]Event, error) {
	f, err := os.Open(l.Path(sessionID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Event{}, nil
		}
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()

	out := []Event{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		// Seq is the first field of every line (Event's field order), so
		// lines behind the cursor are skipped on a prefix read instead of
		// a full decode — polling clients pass a cursor near the tail.
		if after > 0 {
			if seq, ok := leadingSeq(line); ok && seq <= after {
				continue
			}
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			// A torn trailing line (crash mid-write) is dropped, not fatal.
			continue
		}
		if e.Seq <= after {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read event log: %w", err)
	}
	return out, nil
}

// leadingSeq parses the seq from a line that starts with {"seq":N.
func leadingSeq(line []byte) (int64, bool) {
	const prefix = `{"seq":`
	if len(line) <= len(prefix) || string(line[:len(prefix)]) != prefix {
		return 0, false
	}
	var n int64
	i := len(prefix)
	for ; i < len(line) && line[i] >= '0' && line[i] <= '9'; i++ {
		n = n*10 + int64(line[i]-'0')
	}
	if i == len(prefix) {
		return 0, false
	}
	return n, true
}

// lastSeq finds the highest Seq already on disk for a session.
func (l *EventLog) lastSeq(sessionID string) (int64, error) {
	events, err := l.ListEvents(context.Background(), sessionID, 0, 0)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	return events[len(events)-1].Seq, nil
}
