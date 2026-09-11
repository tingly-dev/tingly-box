package e2e_test

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// Journey: every step of a turn is on the record, in order — not just the
// final answer. A real model answers one request with thinking, a
// sentence, a tool call; the CLI emits each block as its own event and
// the transcript must keep all of them, so the UI can fold the process
// and still show it on demand (the way the IM bridge does).
func TestJourney_TurnDetailsRecorded(t *testing.T) {
	requireE2E(t)
	up := newScriptedUpstream(t,
		upstreamTurn{Blocks: []upstreamBlock{
			{Thinking: "The user wants a marker file; I should create it first."},
			{Text: "Let me create the marker file."},
			{Bash: "touch details-marker.txt && echo created-marker"},
		}},
		upstreamTurn{Blocks: []upstreamBlock{
			{Thinking: "The file exists now."},
			{Text: "Created details-marker.txt."},
		}},
	)
	s := bootStack(t, up)
	dir := newGitDir(t, "details")

	d := s.createSession(map[string]any{"local_path": dir, "prompt": "create a marker file", "permission_mode": "bypassPermissions"})
	_, ev := s.waitIdle(d.Session.ID)

	// The order the turn happened in, as the user would read it.
	want := []struct {
		kind managedagent.EventKind
		text string
	}{
		{managedagent.EventThinking, "create it first"},
		{managedagent.EventAssistantMessage, "Let me create the marker file."},
		{managedagent.EventToolUse, "Bash"},
		{managedagent.EventToolResult, "created-marker"},
		{managedagent.EventThinking, "exists now"},
		{managedagent.EventAssistantMessage, "Created details-marker.txt."},
	}
	i := 0
	for _, e := range ev {
		if i < len(want) && e.Kind == want[i].kind && contains(e.Text, want[i].text) {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("turn details incomplete: matched %d of %d in order\n%s", i, len(want), eventsDump(ev))
	}
	if n := count(ev, managedagent.EventAssistantMessage); n != 2 {
		t.Fatalf("want exactly 2 assistant messages (intermediate + final), got %d\n%s", n, eventsDump(ev))
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func count(ev []managedagent.Event, kind managedagent.EventKind) int {
	n := 0
	for _, e := range ev {
		if e.Kind == kind {
			n++
		}
	}
	return n
}
