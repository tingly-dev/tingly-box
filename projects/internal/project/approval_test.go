package project

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A tool's own request reaches the user as an ordinary approval card: it is in
// the thread's pending requests (so the web and IM show it), the call waits for
// the answer, "allow for this thread" is remembered for that exact request, and
// a no answer is returned with the user's words.
func TestToolApprovalIsAnApprovalCardTheCallWaitsOn(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Stack", Task: "x"})
	require.NoError(t, err)
	f.report(t)

	ask := Approval{Tool: "mcp__tingly-projects__create_pr", Summary: "Push branch b", Reason: "branch b belongs to another thread", Input: map[string]any{"head": "b"}, Key: "k"}
	type result struct {
		ok  bool
		why string
		err error
	}
	answered := func(ans Answer) result {
		out := make(chan result, 1)
		go func() {
			ok, why, err := f.threads.askApproval(context.Background(), th.ID, ask)
			out <- result{ok, why, err}
		}()
		var pending []db.ThreadEvent
		require.Eventually(t, func() bool { pending = f.threads.PendingRequests(th.ID); return len(pending) == 1 }, 5*time.Second, 5*time.Millisecond)
		assert.Equal(t, db.EvApproval, pending[0].Kind)
		assert.Contains(t, pending[0].Payload, "branch b belongs to another thread")
		assert.Equal(t, db.ThreadWaiting, func() string { cur, _ := f.store.GetThread(th.ID); return cur.Status }())
		select {
		case <-out:
			t.Fatal("the call returned before the user answered")
		case <-time.After(50 * time.Millisecond):
		}
		var p requestPayload
		require.NoError(t, jsonUnmarshal(pending[0].Payload, &p))
		require.NoError(t, f.threads.Respond(th.ID, p.RequestID, ans))
		return <-out
	}

	r := answered(Answer{Allow: false, Answer: "not that one"})
	require.NoError(t, r.err)
	assert.False(t, r.ok)
	assert.Equal(t, "not that one", r.why)

	r = answered(Answer{Allow: true})
	require.NoError(t, r.err)
	assert.True(t, r.ok)
	// Allowed once: the next call asks again.
	r = answered(Answer{Allow: true, Scope: "thread"})
	require.True(t, r.ok)

	// Allowed for the thread: the same request goes through with no card.
	ok, _, err := f.threads.askApproval(context.Background(), th.ID, ask)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, f.threads.PendingRequests(th.ID))
	// Another request (another key) still asks.
	other := ask
	other.Key = "k2"
	go func() { _, _, _ = f.threads.askApproval(context.Background(), th.ID, other) }()
	require.Eventually(t, func() bool { return len(f.threads.PendingRequests(th.ID)) == 1 }, 5*time.Second, 5*time.Millisecond)
	for _, ev := range f.threads.PendingRequests(th.ID) {
		var p requestPayload
		require.NoError(t, jsonUnmarshal(ev.Payload, &p))
		require.NoError(t, f.threads.Respond(th.ID, p.RequestID, Answer{Allow: false}))
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
