package project

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// threadStore is the session engine's view of the threads: it turns
// transcript entries into thread events and status changes into the
// thread's state. Turn outcomes are settled in Threads.turnEnded, which
// has the whole turn in hand.
type threadStore struct{ m *Threads }

// SetRunning marks the thread working: a turn started, either one the
// thread was sent or one Claude started by itself to follow up on a
// background task.
func (s threadStore) SetRunning(sid string) bool {
	id := s.m.threadOf(sid)
	s.m.mu.Lock()
	if _, ok := s.m.turns[id]; !ok {
		s.m.turns[id] = &turnTally{}
	}
	s.m.mu.Unlock()
	_, err := s.m.update(id, func(t *db.Thread) { t.Status, t.WaitReason = db.ThreadWorking, "" })
	return err == nil
}

func (threadStore) SetCompleted(string, string) bool { return true }
func (threadStore) SetFailed(string, string) bool    { return true }
func (threadStore) SetInterrupted(string)            {}

// Engine notes the thread view says its own way ("Stopped.") or not at all.
var quietNotes = []string{"claude code session ", "interrupted; send a message to resume"}

// Append records an entry as a thread event. A subagent's own steps are kept
// on every thread, tagged with their parent call: the page reads them apart
// from the conversation, when the subagent is opened. So are task events,
// which say how a subagent stands. A folder session also keeps usage events,
// because its page draws them.
func (s threadStore) Append(sid string, e engine.Entry) {
	id := s.m.threadOf(sid)
	t, err := s.m.store.GetThread(id)
	if err != nil {
		return
	}
	session := s.m.isFolder(t.ProjectID)
	var ev *db.ThreadEvent
	switch {
	case e.Role == "user":
		// The thread recorded it, with who sent it, when it was queued.
	case e.Role == "assistant":
		if e.Parent == "" {
			// A subagent's text is not the thread's reply.
			s.tally(id, func(t *turnTally) { t.text = e.Content })
		}
		ev = &db.ThreadEvent{Kind: db.EvAssistant, Content: e.Content}
	case e.Kind == engine.KindToolUse:
		ev = &db.ThreadEvent{Kind: db.EvToolUse, Content: e.Content, Payload: string(e.Payload), RequestID: e.RequestID}
	case e.Kind == engine.KindToolResult:
		ev = &db.ThreadEvent{Kind: db.EvToolResult, Content: e.Content, Payload: string(e.Payload), RequestID: e.RequestID}
	case e.Kind == engine.KindUsage:
		var u engine.TurnUsage
		if json.Unmarshal(e.Payload, &u) == nil {
			s.tally(id, func(t *turnTally) {
				t.inputTokens += u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
				t.outputTokens += u.OutputTokens
				if u.Model != "" {
					t.model = u.Model
				}
			})
		}
		if session {
			ev = &db.ThreadEvent{Kind: db.EvUsage, Payload: string(e.Payload)}
		}
	case e.Kind == engine.KindTask:
		// Every thread keeps task events: a subagent's card reads its state
		// from them. A project thread also gets a line when one ends.
		s.m.addEvent(t, &db.ThreadEvent{Kind: db.EvTask, Payload: string(e.Payload), RequestID: e.RequestID, Parent: e.Parent})
		if note := taskNote(e); note != "" && !session {
			ev = &db.ThreadEvent{Kind: db.EvSystem, Content: note}
		}
	case e.Kind == engine.KindError:
		ev = &db.ThreadEvent{Kind: db.EvError, Content: e.Content}
	case e.Kind == engine.KindSystem:
		for _, q := range quietNotes {
			if strings.HasPrefix(e.Content, q) {
				return
			}
		}
		ev = &db.ThreadEvent{Kind: db.EvSystem, Content: e.Content}
	}
	if ev == nil {
		return
	}
	ev.Parent = e.Parent
	s.m.addEvent(t, ev)
	if ev.Parent == "" && (ev.Kind == db.EvAssistant || ev.Kind == db.EvToolUse) && s.m.OnStep != nil {
		s.m.OnStep(*t, *ev)
	}
}

func (s threadStore) tally(id string, fn func(*turnTally)) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if t, ok := s.m.turns[id]; ok {
		fn(t)
	}
}

// taskNote is the line a finished background task leaves in the thread.
// Its progress stays out: a thread is read for what it did.
func taskNote(e engine.Entry) string {
	var ev engine.TaskEvent
	if json.Unmarshal(e.Payload, &ev) != nil || ev.Event != "task_notification" || ev.Status == "" {
		return ""
	}
	what := ev.Description
	if what == "" {
		what = "Background task"
	}
	note := fmt.Sprintf("%s: %s", what, ev.Status)
	if ev.Summary != "" && ev.Summary != ev.Description {
		note += " · " + firstLine(ev.Summary, 160)
	}
	return note
}
