package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorCommandIsOnlyAPersonsTextMessage(t *testing.T) {
	user := func(content, source, payload string) *db.ThreadEvent {
		return &db.ThreadEvent{Kind: db.EvUser, Content: content, Source: source, Payload: payload}
	}
	file := AttachmentsPayload([]Attachment{{Name: "a.txt", Path: "uploads/a.txt"}})
	for name, tc := range map[string]struct {
		ev   *db.ThreadEvent
		want bool
	}{
		"web":                {user("/compact", "", ""), true},
		"im with arguments":  {user("/compact keep it", db.SourceIM, ""), true},
		"relayed by thread":  {user("/compact", db.SourceThread, ""), false},
		"system":             {user("/compact", db.SourceSystem, ""), false},
		"with a file":        {user("/compact", "", file), false},
		"plain text":         {user("compact please", "", ""), false},
		"a report":           {&db.ThreadEvent{Kind: db.EvThreadUpdate, Content: "/compact"}, false},
		"not compact":        {user("/clear", "", ""), false},
		"no event":           {nil, false},
		"a system check-in":  {&db.ThreadEvent{Kind: db.EvSystem, Content: "/compact"}, false},
		"assistant spelling": {&db.ThreadEvent{Kind: db.EvAssistant, Content: "/compact"}, false},
	} {
		_, got := coordinatorCommand(tc.ev)
		assert.Equal(t, tc.want, got, name)
	}
}

type coordinatorCompactFixture struct {
	svc     *Service
	store   *db.Store
	project *db.Project
	mu      sync.Mutex
	prompts []string
	finals  []db.ThreadEvent
	refuse  string
}

func newCoordinatorCompactFixture(t *testing.T) *coordinatorCompactFixture {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	f := &coordinatorCompactFixture{store: store}
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		f.mu.Lock()
		f.prompts = append(f.prompts, turn.Prompt)
		refuse := f.refuse
		f.mu.Unlock()
		turn.Init("default-model")
		if strings.HasPrefix(turn.Prompt, "/compact") {
			if refuse != "" {
				turn.CompactRefused(refuse)
				return nil
			}
			turn.Compacted(agentboot.CompactResult{Trigger: agentboot.CompactManual, TokensBefore: 120000, TokensAfter: 18000, DurationMS: 4200,
				Reported: agentboot.CompactReported{Tokens: true}})
			return nil
		}
		turn.Say("done", 30, 4)
		return nil
	}}
	f.svc = NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, coordinatorSessionTestHost{fakeLLM: &fakeLLM{}})
	f.svc.Coordinator.OnFinal = func(e db.ThreadEvent) {
		f.mu.Lock()
		f.finals = append(f.finals, e)
		f.mu.Unlock()
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		f.svc.Threads.Shutdown(ctx)
		_ = f.svc.CloseTools(ctx)
	})
	f.project, err = f.svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	return f
}

func (f *coordinatorCompactFixture) say(t *testing.T, text string, replies int) {
	t.Helper()
	_, err := f.svc.Send(context.Background(), f.project.MainThreadID, text, nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
		return countKind(events, db.EvAssistant) == replies && !f.svc.Coordinator.Busy(f.project.ID)
	}, 5*time.Second, 10*time.Millisecond)
}

func (f *coordinatorCompactFixture) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func (f *coordinatorCompactFixture) card(t *testing.T) commandPayload {
	t.Helper()
	var pl commandPayload
	require.Eventually(t, func() bool {
		events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
		cards := 0
		for _, e := range events {
			if e.Kind == db.EvCommand {
				cards++
				pl = commandPayload{}
				_ = json.Unmarshal([]byte(e.Payload), &pl)
			}
		}
		return cards == 1 && (pl.State == "ok" || pl.State == "failed") && !f.svc.Coordinator.Busy(f.project.ID)
	}, 5*time.Second, 10*time.Millisecond)
	return pl
}

func TestCoordinatorCompactsItsOwnSession(t *testing.T) {
	f := newCoordinatorCompactFixture(t)
	f.say(t, "hello", 1)

	_, err := f.svc.Send(context.Background(), f.project.MainThreadID, "/compact keep the decisions", nil, Source{})
	require.NoError(t, err)
	pl := f.card(t)

	assert.Equal(t, commandPayload{Name: "compact", State: "ok", Trigger: "manual", TokensBefore: 120000, TokensAfter: 18000, TokensReported: true, DurationMS: 4200}, pl)
	prompts := f.sent()
	require.Len(t, prompts, 2)
	assert.Equal(t, "/compact keep the decisions", prompts[1], "bare command, no envelope or context around it")
	events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
	assert.Equal(t, 1, countKind(events, db.EvAssistant), "the compaction answered nothing")
	main, err := f.store.GetThread(f.project.MainThreadID)
	require.NoError(t, err)
	assert.Equal(t, 1, main.Turns, "a compaction is not a conversation turn")
	assert.EqualValues(t, 18000, main.ContextTokens)

	f.mu.Lock()
	var heard []string
	for _, e := range f.finals {
		heard = append(heard, e.Content)
	}
	f.mu.Unlock()
	assert.Contains(t, heard, "Compacted: 120k → 18k tokens.", "the chats bound to the conversation hear the outcome")

	// The next message gets the project context again, in case the summary dropped it.
	f.say(t, "what now?", 2)
	prompts = f.sent()
	require.Len(t, prompts, 3)
	assert.Contains(t, prompts[2], "Current project context")
	f.say(t, "and then?", 3)
	assert.NotContains(t, f.sent()[3], "Current project context", "only the first turn after a compaction")
}

func TestCoordinatorCompactRefusedDoesNotBlockTheQueue(t *testing.T) {
	f := newCoordinatorCompactFixture(t)
	f.say(t, "hello", 1)
	f.mu.Lock()
	f.refuse = "Not enough messages to compact."
	f.mu.Unlock()

	_, err := f.svc.Send(context.Background(), f.project.MainThreadID, "/compact", nil, Source{})
	require.NoError(t, err)
	pl := f.card(t)
	assert.Equal(t, "failed", pl.State)
	assert.Equal(t, "Not enough messages to compact.", pl.Reason)
	events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
	assert.Equal(t, 0, countKind(events, db.EvError), "a refusal is a card, not an error")

	f.say(t, "still there?", 2)
}

func TestCoordinatorHasNothingToCompactBeforeItsFirstTurn(t *testing.T) {
	f := newCoordinatorCompactFixture(t)
	_, err := f.svc.Send(context.Background(), f.project.MainThreadID, "/compact", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !f.svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	assert.Empty(t, f.sent(), "no session was started")
	events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
	var notes []string
	for _, e := range events {
		if e.Kind == db.EvSystem {
			notes = append(notes, e.Content)
		}
	}
	assert.Contains(t, notes, "nothing to compact yet: this conversation has not started.")
}
