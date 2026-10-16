package project

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestImportSessionsOnceAndResume(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Back again.", 1, 1)
		return nil
	})
	folder := t.TempDir()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	input, _ := json.Marshal(map[string]any{"command": "npm test"})
	open := ImportedSession{
		SessionID: "cc-1", Folder: folder, PermissionMode: "plan", Profile: "p1", Model: "opus",
		CreatedAt: at, UpdatedAt: at.Add(time.Minute),
		Entries: []engine.Entry{
			{Role: "user", Content: "run the tests", Timestamp: at},
			{Kind: engine.KindThinking, Content: "hmm", Timestamp: at},
			{Kind: engine.KindApprovalRequest, RequestID: "r1", Content: "Bash", Payload: input, Timestamp: at},
			{Kind: engine.KindApprovalResponse, RequestID: "r1", Content: "approved", Timestamp: at},
			{Kind: engine.KindAskRequest, RequestID: "r2", Content: "Which suite?", Timestamp: at},
			{Role: "assistant", Content: "All green.", Timestamp: at},
		},
	}
	closed := ImportedSession{SessionID: "cc-2", Folder: folder, Task: "old work", Closed: true, CreatedAt: at, UpdatedAt: at}

	n, err := f.threads.ImportSessions("desk", []ImportedSession{open, closed})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = f.threads.ImportSessions("desk", []ImportedSession{open, {SessionID: "cc-3", Folder: folder}})
	require.NoError(t, err)
	assert.Zero(t, n, "a source imports once")

	sessions, _ := f.store.ListSessions(folder)
	require.Len(t, sessions, 2)
	var s, old db.Thread
	for _, x := range sessions {
		if x.SessionID == "cc-1" {
			s = x
		} else {
			old = x
		}
	}
	assert.Equal(t, "run the tests", s.Title)
	assert.Equal(t, db.ThreadIdle, s.Status)
	assert.Equal(t, 1, s.Turns)
	assert.Equal(t, "p1", s.Profile)
	assert.True(t, s.UpdatedAt.Equal(at.Add(time.Minute)), "keeps its place in the list")
	assert.Equal(t, db.ThreadResolved, old.Status)

	events, _ := f.store.ListThreadEvents(s.ID)
	kinds := []string{}
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	assert.Equal(t, []string{db.EvUser, db.EvApproval, db.EvAsk, db.EvAssistant}, kinds)
	assert.Contains(t, events[1].Payload, `"state":"allowed"`)
	assert.Contains(t, events[2].Payload, `"state":"expired"`)

	// The next message resumes the same Claude Code session.
	_, err = f.threads.Send(context.Background(), s.ID, "and again", "user")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(f.agent.Launches()) == 1 }, 5*time.Second, 10*time.Millisecond)
	spec := f.agent.Launches()[0]
	assert.True(t, spec.Resume)
	assert.Equal(t, "cc-1", spec.SessionID)
}
