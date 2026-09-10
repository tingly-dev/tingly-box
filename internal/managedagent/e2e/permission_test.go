package e2e_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// Journey: the agent wants to run a command.
//
//	default mode → approval_request → session waits → user approves →
//	command runs, its output goes back to the model → idle.
//	Then: deny → the model is told, turn still ends idle.
//	Then: switch to bypassPermissions → no question asked.
func TestJourney_PermissionPrompt(t *testing.T) {
	requireE2E(t)
	up := newScriptedUpstream(t,
		upstreamTurn{Bash: "touch approved-marker-1.txt && echo approved-marker-1"}, // turn 1: ask (a write, so the CLI must ask)
		upstreamTurn{Text: "ran it"}, // turn 1: after tool result
	)
	s := bootStack(t, up)
	dir := newGitDir(t, "perm")

	d := s.createSession(map[string]any{"local_path": dir, "prompt": "run the command"})
	id := d.Session.ID
	if d.Session.PermissionMode != "" && d.Session.PermissionMode != managedagent.PermissionDefault {
		t.Fatalf("unexpected mode %q", d.Session.PermissionMode)
	}

	// The question reaches the host and the session waits.
	d, ev := s.waitFor(id, turnTimeout, func(d sessionDetail, _ []managedagent.Event) bool {
		return d.Session.Status == managedagent.SessionWaitingInput || d.Session.Status == managedagent.SessionIdle || d.Session.Status == managedagent.SessionFailed
	})
	if d.Session.Status != managedagent.SessionWaitingInput {
		t.Fatalf("no permission prompt; status=%s\n%s\nlast upstream request (tail): %s", d.Session.Status, eventsDump(ev), around(up.LastRequestJSON(), "tool_result", 900))
	}
	req := findEvent(ev, managedagent.EventApprovalRequest)
	if req == nil || req.RequestID == "" || !strings.Contains(string(req.Payload), "approved-marker-1") {
		t.Fatalf("approval request missing or incomplete\n%s", eventsDump(ev))
	}
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/respond", map[string]any{"request_id": req.RequestID, "approved": true}, nil); code != 202 {
		t.Fatalf("respond: %d", code)
	}
	d, ev = s.waitIdle(id)
	if !hasEvent(ev, managedagent.EventApprovalResponse, "") || !hasEvent(ev, managedagent.EventToolResult, "approved-marker-1") {
		t.Fatalf("approval/tool result missing\n%s", eventsDump(ev))
	}
	if _, err := os.Stat(filepath.Join(dir, "approved-marker-1.txt")); err != nil {
		t.Fatalf("approved command did not run: %v", err)
	}
	if !strings.Contains(up.LastRequestJSON(), "approved-marker-1") {
		t.Fatal("the command output never went back to the model")
	}
	if !hasEvent(ev, managedagent.EventAssistantMessage, "ran it") {
		t.Fatalf("final answer missing\n%s", eventsDump(ev))
	}
	t.Logf("approve path ok\n%s", eventsDump(ev))

	// Deny: the model hears it was refused; the turn still ends cleanly.
	before := len(ev)
	up.Queue(upstreamTurn{Bash: "touch denied-marker-2.txt && echo denied-marker-2"}, upstreamTurn{Text: "understood, not running it"})
	s.send(id, "try again")
	d, ev = s.waitStatus(id, managedagent.SessionWaitingInput)
	req = nil
	for i := before; i < len(ev); i++ {
		if ev[i].Kind == managedagent.EventApprovalRequest {
			req = &ev[i]
		}
	}
	if req == nil {
		t.Fatalf("second approval request missing\n%s", eventsDump(ev[before:]))
	}
	if code := s.do(http.MethodPost, "/api/v1/agent/sessions/"+id+"/respond", map[string]any{"request_id": req.RequestID, "approved": false, "answer": "no thanks"}, nil); code != 202 {
		t.Fatalf("respond deny: %d", code)
	}
	d, ev = s.waitNextTurnSettled(id, before)
	if d.Session.Status != managedagent.SessionIdle {
		t.Fatalf("deny turn: %s %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev[before:]))
	}
	if _, err := os.Stat(filepath.Join(dir, "denied-marker-2.txt")); err == nil {
		t.Fatalf("denied command ran anyway\n%s", eventsDump(ev[before:]))
	}
	if !hasEvent(ev[before:], managedagent.EventAssistantMessage, "understood") {
		t.Fatalf("model did not get the denial\n%s", eventsDump(ev[before:]))
	}
	t.Logf("deny path ok\n%s", eventsDump(ev[before:]))

	// bypassPermissions: nothing to answer; the log still shows what ran.
	if code := s.do(http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", map[string]any{"permission_mode": "bypassPermissions"}, &d); code != 200 || d.Session.PermissionMode != managedagent.PermissionBypassPermissions {
		t.Fatalf("set mode: %d %+v", code, d.Session.PermissionMode)
	}
	before = len(ev)
	up.Queue(upstreamTurn{Bash: "touch bypass-marker-3.txt && echo bypass-marker-3"}, upstreamTurn{Text: "done without asking"})
	s.send(id, "go")
	d, ev = s.waitNextTurnSettled(id, before)
	if d.Session.Status != managedagent.SessionIdle {
		t.Fatalf("bypass turn: %s %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev[before:]))
	}
	if _, err := os.Stat(filepath.Join(dir, "bypass-marker-3.txt")); err != nil {
		t.Fatalf("bypass turn did not run the command\n%s", eventsDump(ev[before:]))
	}
	if !hasEvent(ev[before:], managedagent.EventToolResult, "bypass-marker-3") || !hasEvent(ev[before:], managedagent.EventAssistantMessage, "done without asking") {
		t.Fatalf("bypass turn log incomplete\n%s", eventsDump(ev[before:]))
	}
	for _, e := range ev[before:] {
		if e.Kind == managedagent.EventApprovalRequest {
			// Acceptable only if the host answered it itself.
			if !hasEvent(ev[before:], managedagent.EventApprovalResponse, "bypassPermissions") {
				t.Fatalf("bypass mode still waited on the user\n%s", eventsDump(ev[before:]))
			}
		}
	}
	t.Logf("bypass path ok\n%s", eventsDump(ev[before:]))

	// An unsupported mode is rejected up front, not discovered at run time.
	if code := s.do(http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", map[string]any{"permission_mode": "yolo"}, nil); code != 400 {
		t.Fatalf("bad mode: want 400, got %d", code)
	}
}

// around returns up to n bytes of s surrounding the first occurrence of sub.
func around(s, sub string, n int) string {
	i := strings.Index(s, sub)
	if i < 0 {
		return "(no " + sub + " in request)"
	}
	lo, hi := i-n/4, i+n
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}
