package project

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// stopFixture is a service whose threads block until stopped ("run"), wait on
// an approval ("approve"), or finish at once (anything else), and the
// coordinator's MCP client.
type stopFixture struct {
	*testFixture
	svc  *Service
	cs   *mcp.ClientSession
	user *db.ThreadEvent
}

func newStopFixture(t *testing.T) *stopFixture {
	t.Helper()
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		switch turn.Prompt {
		case "run":
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		case "approve":
			if _, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make test"}); err != nil {
				return err
			}
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("done", 1, 1)
		return nil
	}, nil)
	cs, _ := connectTools(t, svc, f.project.MainThreadID, false)
	user := svc.Coordinator.addEvent(f.project.ID, &db.ThreadEvent{Kind: db.EvUser, Content: "Stop the export thread, please."})
	return &stopFixture{testFixture: f, svc: svc, cs: cs, user: user}
}

func (f *stopFixture) start(t *testing.T, task string) *db.Thread {
	t.Helper()
	th, err := f.svc.Threads.Start(context.Background(), f.project, StartSpec{Task: task, Repo: "none"})
	require.NoError(t, err)
	return th
}

func (f *stopFixture) stop(t *testing.T, args map[string]any) (string, bool) {
	t.Helper()
	res, err := f.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "stop_thread", Arguments: args})
	require.NoError(t, err)
	var text []string
	for _, c := range res.Content {
		text = append(text, c.(*mcp.TextContent).Text)
	}
	return strings.Join(text, "\n"), res.IsError
}

func (f *stopFixture) notes(t *testing.T, id string) []string {
	t.Helper()
	events, err := f.store.ListThreadEvents(id)
	require.NoError(t, err)
	var out []string
	for _, e := range events {
		if e.Kind == db.EvSystem {
			out = append(out, e.Content)
		}
	}
	return out
}

func TestStopThreadStopsARunningTurn(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "run")
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	// A message queued behind the running turn is dropped with it.
	_, err := f.svc.Threads.Send(context.Background(), th.ID, "queued", Source{}, nil)
	require.NoError(t, err)

	out, isErr := f.stop(t, map[string]any{"thread_id": th.ID[:8], "cite": []string{f.user.ID[:8]}})
	require.False(t, isErr, out)
	assert.Equal(t, "stopped; the thread is now idle", out)
	got := waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.False(t, f.svc.Threads.Running(got.ID))
	require.Eventually(t, func() bool { return len(f.notes(t, th.ID)) > 0 }, 5e9, 10e6)
	assert.Equal(t, []string{"Stopped by the coordinator."}, f.notes(t, th.ID))
}

func TestStopThreadOnAnIdleThreadIsNotAnError(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "quick")
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	require.Eventually(t, func() bool { return !f.svc.Threads.Running(th.ID) }, 5e9, 10e6)
	out, isErr := f.stop(t, map[string]any{"thread_id": th.ID, "cite": []string{f.user.ID}})
	assert.False(t, isErr)
	assert.Equal(t, "not running; nothing to stop", out)
	assert.Empty(t, f.notes(t, th.ID))
}

// A thread waiting on an approval has its turn in flight: stopping it ends the
// wait, the card expires (it can no longer be answered) and the thread idles.
func TestStopThreadExpiresAPendingApproval(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "approve")
	waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	require.Len(t, f.svc.Threads.PendingRequests(th.ID), 1)

	out, isErr := f.stop(t, map[string]any{"thread_id": th.ID[:8], "cite": []string{f.user.ID[:8]}})
	require.False(t, isErr, out)
	t.Log(out)
	assert.Equal(t, "stopped; the thread is now idle", out)
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status)
	assert.Empty(t, got.WaitReason)
	assert.Empty(t, f.svc.Threads.PendingRequests(th.ID))
	events, _ := f.store.ListThreadEvents(th.ID)
	var states []string
	for _, e := range events {
		if e.Kind == db.EvApproval {
			var p struct{ State string }
			require.NoError(t, json.Unmarshal([]byte(e.Payload), &p))
			states = append(states, p.State)
		}
	}
	assert.Equal(t, []string{"expired"}, states)
	assert.ErrorIs(t, f.svc.Threads.Respond(th.ID, "req-1", Answer{Allow: true}), ErrNoPrompt)
}

func TestStopThreadNeedsTheUsersWords(t *testing.T) {
	f := newStopFixture(t)
	th := f.start(t, "run")
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	agent := f.svc.Coordinator.addEvent(f.project.ID, &db.ThreadEvent{Kind: db.EvAssistant, Content: "I will stop it."})
	for name, args := range map[string]map[string]any{
		"no cite":        {"thread_id": th.ID},
		"empty cite":     {"thread_id": th.ID, "cite": []string{}},
		"unknown id":     {"thread_id": th.ID, "cite": []string{"nope"}},
		"not the user's": {"thread_id": th.ID, "cite": []string{agent.ID}},
		"extra property": {"thread_id": th.ID, "cite": []string{f.user.ID}, "why": "x"},
		"one bad of two": {"thread_id": th.ID, "cite": []string{f.user.ID, "nope"}},
	} {
		out, isErr := f.stop(t, args)
		assert.True(t, isErr, "%s: %s", name, out)
	}
	assert.True(t, f.svc.Threads.Running(th.ID), "a refused call stops nothing")
	got, _ := f.store.GetThread(th.ID)
	assert.Equal(t, db.ThreadWorking, got.Status)
	f.svc.Threads.Stop(th.ID, Source{})
}

func TestStopThreadStaysInItsProjectAndOffTheMainThread(t *testing.T) {
	f := newStopFixture(t)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	foreign := &db.Thread{ProjectID: other.ID, ParentID: other.MainThreadID, Agent: db.AgentClaudeCode, Title: "Elsewhere"}
	require.NoError(t, f.store.CreateThread(foreign))
	cite := []string{f.user.ID}
	for name, id := range map[string]string{"other project": foreign.ID, "own main thread": f.project.MainThreadID, "other main thread": other.MainThreadID} {
		out, isErr := f.stop(t, map[string]any{"thread_id": id, "cite": cite})
		assert.True(t, isErr, "%s: %s", name, out)
	}
}
