package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func waitThread(t *testing.T, store *db.Store, id, status string) *db.Thread {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		th, err := store.GetThread(id)
		require.NoError(t, err)
		if th.Status == status {
			return th
		}
		time.Sleep(10 * time.Millisecond)
	}
	th, _ := store.GetThread(id)
	t.Fatalf("thread status %q, want %q (%s)", th.Status, status, th.WaitReason)
	return nil
}

// A chat is created and listed by /chats, its hidden project never appears in
// /projects, its conversation runs on the thread endpoints, and DELETE removes
// it.
func TestChatRoutesAndHiddenProject(t *testing.T) {
	store, c := newServer(t, quietAgents())

	var ch db.Thread
	require.Equal(t, 201, c.do("POST", "/chats", map[string]any{"content": "hello chat"}, &ch))
	assert.Equal(t, db.AgentClaudeCode, ch.Agent)
	assert.Contains(t, ch.Cwd, "/chats/")

	var projects struct {
		Projects []db.Project `json:"projects"`
	}
	require.Equal(t, 200, c.do("GET", "/projects", nil, &projects))
	for _, p := range projects.Projects {
		assert.NotEqual(t, db.ProjectChat, p.Type, "the hidden Chats project is never listed")
	}

	var list struct {
		Chats     []db.Thread `json:"chats"`
		Sandboxed bool        `json:"sandboxed"`
	}
	require.Equal(t, 200, c.do("GET", "/chats", nil, &list))
	require.Len(t, list.Chats, 1)
	assert.Equal(t, ch.ID, list.Chats[0].ID)

	waitThread(t, store, ch.ID, db.ThreadIdle)

	// A second message goes through the ordinary thread endpoint.
	require.Equal(t, 200, c.do("POST", "/threads/"+ch.ID+"/messages", map[string]any{"content": "again"}, &ch))
	waitThread(t, store, ch.ID, db.ThreadIdle)

	// Another project's thread is not a chat.
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	require.Equal(t, 404, c.do("DELETE", "/chats/"+p.MainThreadID, nil, nil))

	require.Equal(t, 200, c.do("DELETE", "/chats/"+ch.ID, nil, nil))
	_, err := store.GetThread(ch.ID)
	require.Error(t, err)
}
