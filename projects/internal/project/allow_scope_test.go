package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestAllowKey(t *testing.T) {
	k1, ok := allowKey("Bash", map[string]any{"command": "  make test "})
	require.True(t, ok)
	k2, _ := allowKey("Bash", map[string]any{"command": "make test"})
	k3, _ := allowKey("Bash", map[string]any{"command": "make test && rm -rf /"})
	assert.Equal(t, k1, k2, "surrounding space does not matter")
	assert.NotEqual(t, k1, k3, "a longer command is another command")

	_, ok = allowKey("Bash", map[string]any{"command": "  "})
	assert.False(t, ok, "nothing to remember without a command")
	_, ok = allowKey("Bash", nil)
	assert.False(t, ok)

	w1, _ := allowKey("Write", map[string]any{"file_path": "/a"})
	w2, _ := allowKey("Write", map[string]any{"file_path": "/b"})
	assert.Equal(t, "Write", w1)
	assert.Equal(t, w1, w2, "other tools are remembered by tool")
}

func TestAllowForThreadCoversOnlyThatBashCommand(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		for i, cmd := range []string{"make test", "make test", "rm -rf build"} {
			resp, err := turn.Approve("req-"+string(rune('1'+i)), "Bash", map[string]any{"command": cmd})
			if err != nil {
				return err
			}
			if !resp.Approved {
				turn.Say("blocked "+cmd, 1, 1)
				return nil
			}
		}
		turn.Say("all ran", 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "build"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: true, Scope: "thread"}))

	require.Eventually(t, func() bool {
		_, ok := f.threads.PendingRequest(th.ID, "req-3")
		return ok
	}, 5*time.Second, 5*time.Millisecond, "req-2 (make test again) went through, a different command asks")
	_, asked := f.threads.PendingRequest(th.ID, "req-2")
	assert.False(t, asked)
	require.NoError(t, f.threads.Respond(th.ID, "req-3", Answer{Allow: false}))
	assert.Equal(t, "blocked rm -rf build", f.report(t).Text)
}

func TestAllowForThreadStillCoversTheWholeToolForOthers(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		for i, path := range []string{"/a.txt", "/b.txt"} {
			resp, err := turn.Approve("req-"+string(rune('1'+i)), "Write", map[string]any{"file_path": path})
			if err != nil {
				return err
			}
			if !resp.Approved {
				turn.Say("blocked "+path, 1, 1)
				return nil
			}
		}
		turn.Say("both written", 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "write"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: true, Scope: "thread"}))
	assert.Equal(t, "both written", f.report(t).Text)
}

func TestBashApprovalButtonSaysItIsForThatCommand(t *testing.T) {
	assert.Equal(t, "Allow this command for this thread", allowThreadLabel("Bash"))
	assert.Equal(t, "Allow for this thread", allowThreadLabel("Write"))
}

func TestPendingRequestsListsWhatTheThreadWaitsOn(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make test"})
		if err != nil {
			return err
		}
		if resp.Approved {
			turn.Say("ran", 1, 1)
		} else {
			turn.Say("blocked", 1, 1)
		}
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "build"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWaiting)

	pending := f.threads.PendingRequests(th.ID)
	require.Len(t, pending, 1)
	assert.Equal(t, db.EvApproval, pending[0].Kind)
	assert.Contains(t, pending[0].Payload, `"request_id":"req-1"`)
	assert.Contains(t, pending[0].Payload, `"state":"pending"`)

	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: false}))
	assert.Empty(t, f.threads.PendingRequests(th.ID), "an answered request is no longer pending")
	assert.NotNil(t, f.threads.PendingRequests("no-such-thread"), "a thread with nothing pending lists an empty set, not null")
}
