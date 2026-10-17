package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ImportedSession is a Claude Code session another host ran in a folder
// (Tingly Box's retired Desk), brought into Projects as a folder session.
// SessionID is Claude Code's own session id, so the next turn resumes it.
type ImportedSession struct {
	SessionID      string
	Folder         string
	Task           string
	Closed         bool
	PermissionMode string
	Profile        string
	Model          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Entries        []engine.Entry
	// Err is set when the host could not read the session: it is skipped,
	// and the import runs again next start.
	Err error
}

// ImportSessions brings sessions in once per source: a later call with the
// same source does nothing, so a session deleted after the import stays
// deleted. A session whose Claude Code id is already known is skipped.
// It returns how many were imported.
func (m *Threads) ImportSessions(source string, in []ImportedSession) (int, error) {
	key := "import." + source
	var done bool
	if ok, err := m.store.GetSetting(key, &done); err != nil {
		return 0, err
	} else if ok && done {
		return 0, nil
	}
	n := 0
	var errs []error
	for _, s := range in {
		if s.Err != nil {
			errs = append(errs, fmt.Errorf("session %s: %w", s.SessionID, s.Err))
			continue
		}
		ok, err := m.importSession(s)
		if err != nil {
			errs = append(errs, fmt.Errorf("session %s: %w", s.SessionID, err))
			continue
		}
		if ok {
			n++
		}
	}
	if len(errs) > 0 {
		// Try again next start; what got in (whole) is skipped then.
		return n, errors.Join(errs...)
	}
	return n, m.store.PutSetting(key, true)
}

func (m *Threads) importSession(s ImportedSession) (bool, error) {
	if s.SessionID == "" || s.Folder == "" {
		return false, nil
	}
	if known, err := m.store.HasSession(s.SessionID); err != nil || known {
		return false, err
	}
	task := strings.TrimSpace(s.Task)
	events, turns := importEvents(s.Entries)
	if task == "" {
		for _, e := range events {
			if e.Kind == db.EvUser {
				task = e.Content
				break
			}
		}
	}
	// Every Desk session ran its first turn when it was created, so the next
	// message resumes it even when its transcript is gone.
	turns = max(turns, 1)
	t := &db.Thread{
		// The thread keeps the session's id, so old links still find it.
		ID:    s.SessionID,
		Title: firstLine(task, 60), Task: task, Status: db.ThreadIdle, Origin: "user",
		Worktree: s.Folder, SessionID: s.SessionID, Turns: turns,
		Model: s.Model, PermissionMode: s.PermissionMode, Profile: s.Profile,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	if t.Title == "" {
		t.Title = "Session"
	}
	if s.Closed {
		at := s.UpdatedAt
		t.Status, t.ResolvedAt = db.ThreadResolved, &at
	}
	if err := m.store.ImportThread(t, events); err != nil {
		return false, err
	}
	_ = m.store.SaveWorkdirAt(s.Folder, s.UpdatedAt)
	return true, nil
}

// importEvents turns a stored transcript into thread events the way a live
// turn records them (threadStore.Append), folding each approval or question
// together with its answer. It also counts the user's turns.
func importEvents(entries []engine.Entry) ([]*db.ThreadEvent, int) {
	answers := map[string]engine.Entry{}
	for _, e := range entries {
		if e.Kind == engine.KindApprovalResponse || e.Kind == engine.KindAskResponse {
			answers[e.RequestID] = e
		}
	}
	var out []*db.ThreadEvent
	turns := 0
	for _, e := range entries {
		var ev *db.ThreadEvent
		switch {
		case e.Kind == "" && e.Role == "user":
			turns++
			ev = &db.ThreadEvent{Kind: db.EvUser, Content: e.Content, From: "user"}
		case e.Kind == "" && e.Role == "assistant":
			ev = &db.ThreadEvent{Kind: db.EvAssistant, Content: e.Content}
		case e.Kind == engine.KindToolUse:
			ev = &db.ThreadEvent{Kind: db.EvToolUse, Content: e.Content, Payload: string(e.Payload), RequestID: e.RequestID}
		case e.Kind == engine.KindToolResult:
			ev = &db.ThreadEvent{Kind: db.EvToolResult, Content: e.Content, Payload: string(e.Payload), RequestID: e.RequestID}
		case e.Kind == engine.KindUsage:
			ev = &db.ThreadEvent{Kind: db.EvUsage, Payload: string(e.Payload)}
		case e.Kind == engine.KindTask:
			ev = &db.ThreadEvent{Kind: db.EvTask, Payload: string(e.Payload), RequestID: e.RequestID}
		case e.Kind == engine.KindError:
			ev = &db.ThreadEvent{Kind: db.EvError, Content: e.Content}
		case e.Kind == engine.KindSystem:
			ev = &db.ThreadEvent{Kind: db.EvSystem, Content: e.Content}
		case e.Kind == engine.KindApprovalRequest:
			var input map[string]any
			_ = json.Unmarshal(e.Payload, &input)
			p := requestPayload{RequestID: e.RequestID, Tool: e.Content, Input: input, State: "expired"}
			if a, ok := answers[e.RequestID]; ok {
				p.State = map[bool]string{true: "allowed", false: "denied"}[a.Content == "approved"]
			}
			ev = &db.ThreadEvent{Kind: db.EvApproval, Content: describeTool(e.Content, input), Payload: jsonString(p)}
		case e.Kind == engine.KindAskRequest:
			var input map[string]any
			_ = json.Unmarshal(e.Payload, &input)
			p := requestPayload{RequestID: e.RequestID, Tool: "AskUserQuestion", Input: input, State: "expired"}
			if a, ok := answers[e.RequestID]; ok {
				p.State, p.Answer = "answered", a.Content
			}
			// Desk kept a question only in its input, as a live ask does.
			q := e.Content
			if q == "" {
				q = describeQuestion(input)
			}
			ev = &db.ThreadEvent{Kind: db.EvAsk, Content: q, Payload: jsonString(p)}
		}
		if ev == nil {
			continue
		}
		ev.Parent, ev.CreatedAt = e.Parent, e.Timestamp
		out = append(out, ev)
	}
	return out, turns
}
