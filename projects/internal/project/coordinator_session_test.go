package project

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorSessionResumesAcrossMessages(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("default-model"); turn.Say("done", 1, 1); return nil }}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := svc.Coordinator.Post(p.ID, "hello", nil, Source{})
		require.NoError(t, err)
		want := i + 1
		require.Eventually(t, func() bool {
			events, _ := store.ListThreadEvents(p.MainThreadID)
			return countKind(events, db.EvAssistant) == want && !svc.Coordinator.Busy(p.ID)
		}, 5*time.Second, 10*time.Millisecond)
	}
	launches := agent.Launches()
	require.Len(t, launches, 2)
	require.False(t, launches[0].Resume)
	require.True(t, launches[1].Resume)
	require.Equal(t, launches[0].SessionID, launches[1].SessionID)
	require.NotEmpty(t, launches[0].MCPConfigPath)
}

func TestCoordinatorTurnsAddUpOnTheMainThread(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("default-model"); turn.Say("done", 30, 4); return nil }}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := svc.Coordinator.Post(p.ID, "hello", nil, Source{})
		require.NoError(t, err)
		want := i + 1
		require.Eventually(t, func() bool {
			events, _ := store.ListThreadEvents(p.MainThreadID)
			return countKind(events, db.EvAssistant) == want && !svc.Coordinator.Busy(p.ID)
		}, 5*time.Second, 10*time.Millisecond)
	}
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	require.Equal(t, 2, main.Turns)
	require.EqualValues(t, 60, main.InputTokens)
	require.EqualValues(t, 8, main.OutputTokens)
	require.EqualValues(t, 30, main.ContextTokens)
	require.NotNil(t, main.LastTurnAt)
	require.Equal(t, "default-model", main.ReportedModel)
}

// A report turn ends with the system's check-in, which carries the reply
// budget the coordinator prompt refers to.
func TestCoordinatorReportTurnCarriesTheReplyBudget(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		turn.Say("Done.", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		seen.add(c)
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		c.Call("start_thread", map[string]string{"title": "Do it", "task": "do it"})
		c.Reply("Started.")
		return nil
	})
	_, err := svc.Coordinator.Post(f.project.ID, "Please do it.", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 2 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)

	turns := seen.all()
	assert.NotContains(t, turns[0], "earlier messages exist", "a short history is shown whole")
	report := strings.TrimSpace(turns[1])
	last := report[strings.LastIndex(report, "<event"):]
	assert.True(t, strings.HasSuffix(report, "</event>"), "the check-in comes last")
	assert.Contains(t, last, `kind="check-in"`)
	assert.Contains(t, last, `trust="system"`)
	assert.Contains(t, last, `replies_since_user="1"`)
	assert.Contains(t, last, `minutes_since_user="0"`)
	assert.Contains(t, report, "Done.", "the report itself precedes it")
}

// A new session is given the latest part of a long conversation and told
// that more can be read with read_conversation.
func TestCoordinatorSessionSaysWhenHistoryIsCut(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	for i := 0; i < historyLimit+5; i++ {
		require.NoError(t, f.store.AddThreadEvent(&db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Content: fmt.Sprintf("m%03d", i)}))
	}
	_, err := svc.Coordinator.Post(f.project.ID, "And now?", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)

	first := seen.all()[0]
	assert.Contains(t, first, "earlier messages exist")
	assert.Contains(t, first, "read_conversation")
	assert.Contains(t, first, fmt.Sprintf("m%03d", historyLimit+4))
	assert.NotContains(t, first, "m000", "the oldest messages are left to read_conversation")
}

// Stopping a running turn keeps what it already said, asks for no recovery,
// and the queue serves the message sent behind it.
func TestCoordinatorStopKeepsTheReplyAndContinues(t *testing.T) {
	var turns atomic.Int32
	release := make(chan struct{})
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		if turns.Add(1) == 1 {
			c.Use("call-1", "mcp__tingly-projects__list_threads", map[string]any{"limit": 5})
			c.Say("Working on it…", 1, 1)
			select {
			case <-release:
				c.Say("Finished.", 1, 1)
				return nil
			case <-c.Ctx.Done():
				return c.Ctx.Err()
			}
		}
		c.Say("Queued done.", 1, 1)
		return nil
	})
	p := f.project
	_, err := svc.Coordinator.Post(p.ID, "Do the thing.", nil, Source{})
	require.NoError(t, err)
	var live *db.ThreadEvent
	for _, m := range waitMessages(t, f.store, p.ID, func(msgs []db.ThreadEvent) bool {
		return countKind(msgs, db.EvAssistant) == 1
	}) {
		if m.Kind == db.EvAssistant {
			live = &m
		}
	}
	require.NotNil(t, live)
	require.Equal(t, "Working on it…", live.Content)

	// A message behind the running turn must not be dropped by the stop.
	_, err = svc.Coordinator.Post(p.ID, "And this too.", nil, Source{})
	require.NoError(t, err)

	require.NoError(t, svc.Coordinator.StopTurn(p.ID))
	close(release)
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(p.ID) }, 5*time.Second, 10*time.Millisecond)

	msgs, err := f.store.ListThreadEvents(p.MainThreadID)
	require.NoError(t, err)
	// The interrupted turn's tool call and partial reply stay.
	assert.Contains(t, kinds(msgs), db.EvToolUse)
	var partial *db.ThreadEvent
	var follow *db.ThreadEvent
	for i := range msgs {
		switch {
		case msgs[i].Kind == db.EvAssistant && live != nil && msgs[i].ID == live.ID:
			partial = &msgs[i]
		case msgs[i].Kind == db.EvAssistant && (live == nil || msgs[i].ID != live.ID):
			follow = &msgs[i]
		}
	}
	require.NotNil(t, partial, "the stopped turn's live reply is still there")
	assert.Equal(t, "Working on it…", partial.Content, "what the turn said before the stop is kept")
	require.NotNil(t, follow, "the message behind the stopped turn was answered")
	assert.Equal(t, "Queued done.", follow.Content)
	// Nothing is uncertain: no recovery is offered, no error line added.
	recovery, err := f.store.UncertainCoordinatorInput(p.ID)
	require.NoError(t, err)
	assert.Empty(t, recovery)
	assert.Equal(t, 0, countKind(msgs, db.EvError))
}

// Stopping while nothing runs is a no-op: the next message is still answered.
func TestCoordinatorStopWhenIdleLeavesTheNextMessageAlone(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		c.Reply("Answered.")
		return nil
	})
	p := f.project
	_, err := svc.Coordinator.Post(p.ID, "First.", nil, Source{})
	require.NoError(t, err)
	waitMessages(t, f.store, p.ID, func(msgs []db.ThreadEvent) bool {
		return !svc.Coordinator.Busy(p.ID) && countKind(msgs, db.EvAssistant) == 1
	})

	require.NoError(t, svc.Coordinator.StopTurn(p.ID))
	_, err = svc.Coordinator.Post(p.ID, "Second.", nil, Source{})
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, p.ID, func(msgs []db.ThreadEvent) bool {
		return !svc.Coordinator.Busy(p.ID) && countKind(msgs, db.EvAssistant) == 2
	})
	assert.Equal(t, "Answered.", msgs[len(msgs)-1].Content)
	assert.Equal(t, 0, countKind(msgs, db.EvError))
}

// The turn's tool calls are recorded on the conversation like any thread's
// transcript, not just the final text.
func TestCoordinatorSessionRecordsToolCalls(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		c.Use("call-1", "mcp__tingly-projects__start_thread", map[string]any{"title": "Ship it", "task": "ship it"})
		c.Say("Started a thread.", 1, 1)
		return nil
	})
	p := f.project
	_, err := svc.Coordinator.Post(p.ID, "Start work.", nil, Source{})
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, p.ID, func(msgs []db.ThreadEvent) bool {
		return countKind(msgs, db.EvToolUse) == 1 && countKind(msgs, db.EvAssistant) == 1 && !svc.Coordinator.Busy(p.ID)
	})
	var call *db.ThreadEvent
	for i := range msgs {
		if msgs[i].Kind == db.EvToolUse {
			call = &msgs[i]
		}
	}
	require.NotNil(t, call)
	assert.Equal(t, "mcp__tingly-projects__start_thread", call.Content)
	assert.JSONEq(t, `{"title": "Ship it", "task": "ship it"}`, call.Payload)
}

func kinds(msgs []db.ThreadEvent) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Kind)
	}
	return out
}
