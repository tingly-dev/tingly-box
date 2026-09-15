package e2e_test

import (
	"net/http"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// Journey: one folder, two tasks at the same time — the way two `claude`
// sessions run in one directory locally. Both work, each is its own
// conversation with its own Claude Code session, and steering one does not
// touch the other.
func TestJourney_TwoTasksInOneFolder(t *testing.T) {
	requireE2E(t)
	// No script: the upstream answers its fixed text to whichever session
	// asks, so the two turns cannot race into a fixed order.
	up := newScriptedUpstream(t)
	s := bootStack(t, up)
	dir := newGitDir(t, "shared")

	first := s.createSession(map[string]any{"path": dir, "prompt": "task one"})
	second := s.createSession(map[string]any{"path": dir, "prompt": "task two"})
	if first.Folder.ID != second.Folder.ID {
		t.Fatalf("both tasks must be in the same folder: %s vs %s", first.Folder.ID, second.Folder.ID)
	}

	d1, ev1 := s.waitIdle(first.Session.ID)
	d2, ev2 := s.waitIdle(second.Session.ID)
	if d1.Session.CCSessionID == "" || d1.Session.CCSessionID == d2.Session.CCSessionID {
		t.Fatalf("each task needs its own claude session: %q vs %q", d1.Session.CCSessionID, d2.Session.CCSessionID)
	}
	if !hasEvent(ev1, managedagent.EventUserMessage, "task one") || hasEvent(ev1, managedagent.EventUserMessage, "task two") {
		t.Fatalf("transcripts leaked into each other\n%s", eventsDump(ev1))
	}
	if !hasEvent(ev2, managedagent.EventUserMessage, "task two") || hasEvent(ev2, managedagent.EventUserMessage, "task one") {
		t.Fatalf("transcripts leaked into each other\n%s", eventsDump(ev2))
	}

	// Steering one leaves the other alone.
	before2 := len(ev2)
	before1 := len(ev1)
	s.send(first.Session.ID, "only for task one")
	s.waitNextTurnSettled(first.Session.ID, before1)
	if ev := s.events(second.Session.ID); len(ev) != before2 {
		t.Fatalf("steering one task changed the other's log\n%s", eventsDump(ev[before2:]))
	}

	// Both show up as active tasks in the same folder.
	var list struct {
		Sessions []struct {
			Session managedagent.Session `json:"session"`
			Folder  *struct {
				ID string `json:"id"`
			} `json:"folder"`
		} `json:"sessions"`
	}
	if code := s.do(http.MethodGet, "/api/v1/agent/sessions?active=true", nil, &list); code != 200 || len(list.Sessions) != 2 {
		t.Fatalf("active sessions: %d %+v", code, list.Sessions)
	}
	for _, row := range list.Sessions {
		if row.Folder == nil || row.Folder.ID != first.Folder.ID {
			t.Fatalf("both rows must name the shared folder: %+v", row)
		}
	}

	// The folder cannot be withdrawn while they run; once both are archived
	// it can, and the directory is untouched.
	if code := s.do(http.MethodDelete, "/api/v1/agent/folders/"+first.Folder.ID, nil, nil); code != 409 {
		t.Fatalf("removing a folder with running tasks: want 409, got %d", code)
	}
	for _, id := range []string{first.Session.ID, second.Session.ID} {
		if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", nil, nil); code != 200 {
			t.Fatalf("archive %s: %d", id, code)
		}
	}
	if code := s.do(http.MethodDelete, "/api/v1/agent/folders/"+first.Folder.ID, nil, nil); code != 204 {
		t.Fatalf("remove folder: %d", code)
	}
}
