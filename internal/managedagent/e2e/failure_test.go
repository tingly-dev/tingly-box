package e2e_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// Journey: the model errors; the user sees why and can retry in place.
//
//	upstream 400 → session failed with a reason → send again → running →
//	idle. The failed session is not a dead end.
func TestJourney_FailureThenRetry(t *testing.T) {
	requireE2E(t)
	up := newScriptedUpstream(t, upstreamTurn{Status: 400})
	s := bootStack(t, up)
	dir := newGitDir(t, "flaky")

	d := s.createSession(map[string]any{"local_path": dir, "prompt": "hello"})
	id := d.Session.ID
	d, ev := s.waitSettled(id)
	if d.Session.Status != managedagent.SessionFailed {
		t.Fatalf("expected failed, got %s\n%s", d.Session.Status, eventsDump(ev))
	}
	if d.Session.Error == "" {
		t.Fatalf("failed without a reason\n%s", eventsDump(ev))
	}
	t.Logf("failure surfaced as: %s", d.Session.Error)

	// Retry from the failed state; the folder is still there.
	before := len(ev)
	up.Queue(upstreamTurn{Text: "recovered"})
	s.send(id, "try again")
	d, ev = s.waitNextTurnSettled(id, before)
	if d.Session.Status != managedagent.SessionIdle || !hasEvent(ev[before:], managedagent.EventAssistantMessage, "recovered") {
		t.Fatalf("retry: %s %q\n%s", d.Session.Status, d.Session.Error, eventsDump(ev[before:]))
	}
	if d.Session.Error != "" {
		t.Fatalf("stale error kept after recovery: %q", d.Session.Error)
	}
}

// Journey: every advertised permission mode starts a turn on the installed
// CLI. (A mode the CLI rejects must surface as a failed session with the
// CLI's own stderr, which is what the failed status carries.)
func TestJourney_AllPermissionModesStart(t *testing.T) {
	requireE2E(t)
	s := bootStack(t, nil)
	dir := newGitDir(t, "modes")

	var envs struct {
		PermissionModes []string `json:"permission_modes"`
	}
	s.do(http.MethodGet, "/api/v1/agent/environments", nil, &envs)
	if len(envs.PermissionModes) == 0 || !strings.Contains(strings.Join(envs.PermissionModes, ","), "bypassPermissions") {
		t.Fatalf("permission modes not advertised: %v", envs.PermissionModes)
	}
	// Every advertised mode must at least start a turn without the CLI
	// rejecting the flag outright.
	for _, mode := range envs.PermissionModes {
		d := s.createSession(map[string]any{"local_path": dir, "prompt": "hi", "permission_mode": mode})
		d, ev := s.waitSettled(d.Session.ID)
		if d.Session.Status != managedagent.SessionIdle {
			t.Errorf("mode %s: %s %q\n%s", mode, d.Session.Status, d.Session.Error, eventsDump(ev))
		}
	}
}
