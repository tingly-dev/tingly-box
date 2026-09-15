package e2e_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// Journey: stop a turn that is taking too long, then carry on.
//
//	slow model → interrupt → idle ("interrupted"), not failed →
//	next message resumes the same Claude session → answer arrives →
//	archive → no further steering.
func TestJourney_InterruptThenResume(t *testing.T) {
	requireE2E(t)
	up := newScriptedUpstream(t, upstreamTurn{Text: "never", Delay: 2 * time.Minute})
	s := bootStack(t, up)
	dir := newGitDir(t, "slow")

	d := s.createSession(map[string]any{"path": dir, "prompt": "think hard"})
	id := d.Session.ID
	s.waitStatus(id, managedagent.SessionRunning)
	// Give the CLI a moment to actually be inside the upstream call.
	s.waitFor(id, turnTimeout, func(sessionDetail, []managedagent.Event) bool { return len(up.Requests()) > 0 })

	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/interrupt", nil, nil); code != 202 {
		t.Fatalf("interrupt: %d", code)
	}
	d, ev := s.waitFor(id, 30*time.Second, func(_ sessionDetail, ev []managedagent.Event) bool { return turnEnded(ev, 0) })
	if d.Session.Status != managedagent.SessionIdle || d.Session.Error != "" {
		t.Fatalf("after interrupt: %s %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev))
	}
	if !hasEvent(ev, managedagent.EventStatus, "interrupted") {
		t.Fatalf("no interrupted status in the log\n%s", eventsDump(ev))
	}
	ccID := d.Session.CCSessionID

	// Resume: the next turn is quick and continues the same task.
	before := len(ev)
	up.Queue(upstreamTurn{Text: "back and quick"})
	s.send(id, "ok, shorter")
	d, ev = s.waitNextTurnSettled(id, before)
	if d.Session.Status != managedagent.SessionIdle || !hasEvent(ev[before:], managedagent.EventAssistantMessage, "back and quick") {
		t.Fatalf("resume after interrupt: %s %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev[before:]))
	}
	// The Claude Code session carries on — unless the interrupt landed
	// before the CLI ever wrote its session file, in which case there is
	// nothing to resume and a fresh one is started. Both are correct; what
	// must never happen is a silent change, so the log says which it was.
	if ccID != "" && d.Session.CCSessionID != ccID && !hasEvent(ev, managedagent.EventSystem, "was never saved") {
		t.Fatalf("the claude session changed with no explanation in the log: %s → %s\n%s", ccID, d.Session.CCSessionID, eventsDump(ev))
	}
	// Either way the transcript is one conversation: the first prompt is
	// still there alongside the resumed answer.
	if !hasEvent(ev, managedagent.EventUserMessage, "think hard") {
		t.Fatalf("the task lost its earlier turns across the interrupt\n%s", eventsDump(ev))
	}

	// Archive is final.
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", nil, &d); code != 200 || d.Session.Status != managedagent.SessionArchived {
		t.Fatalf("archive: %d %s", code, d.Session.Status)
	}
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", map[string]any{"text": "too late"}, nil); code != 409 {
		t.Fatalf("steer after archive: want 409, got %d", code)
	}
}

// Journey: archive while the agent is mid-turn stops the process.
func TestJourney_ArchiveWhileRunning(t *testing.T) {
	requireE2E(t)
	up := newScriptedUpstream(t, upstreamTurn{Text: "never", Delay: 2 * time.Minute})
	s := bootStack(t, up)
	dir := newGitDir(t, "archive-mid-turn")

	d := s.createSession(map[string]any{"path": dir, "prompt": "think hard"})
	id := d.Session.ID
	s.waitFor(id, turnTimeout, func(sessionDetail, []managedagent.Event) bool { return len(up.Requests()) > 0 })
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", nil, &d); code != 200 || d.Session.Status != managedagent.SessionArchived {
		t.Fatalf("archive: %d %s", code, d.Session.Status)
	}
	// It stays archived once the turn unwinds (the launcher must not
	// overwrite the status on its way out).
	time.Sleep(2 * time.Second)
	if d = s.session(id); d.Session.Status != managedagent.SessionArchived {
		t.Fatalf("status after unwind: %s", d.Session.Status)
	}
}
