package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestProjectGitHubAccessIsValidatedAndDefaultsToAllow(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	assert.Empty(t, p.ThreadGitHubAccess, "a new project says nothing, which is allow")

	var out db.Project
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"thread_github_access": "ask"}, &out))
	assert.Equal(t, "ask", out.ThreadGitHubAccess)
	for _, bad := range []any{"yes", "", "always", 1} {
		assert.Equal(t, 400, c.do("PATCH", "/projects/"+p.ID, map[string]any{"thread_github_access": bad}, nil), bad)
	}
	saved, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, "ask", saved.ThreadGitHubAccess, "a rejected value changes nothing")
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"thread_github_access": "allow"}, &out))
	assert.Equal(t, "allow", out.ThreadGitHubAccess)
}

func TestThreadsCarryTheirIssueAndPRLinksOverTheAPI(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	th := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle}
	require.NoError(t, store.CreateThread(th))
	_, err := store.UpsertThreadLink(th.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 12,
		URL: "https://github.com/acme/widgets/pull/12", Title: "Add a", Role: db.LinkOpened, State: db.LinkStateOpen, CI: "passing", MergeState: "ready"})
	require.NoError(t, err)

	var one struct {
		Thread db.Thread `json:"thread"`
	}
	require.Equal(t, 200, c.do("GET", "/threads/"+th.ID, nil, &one))
	require.Len(t, one.Thread.Links, 1)
	assert.Equal(t, 12, one.Thread.Links[0].Number)
	assert.Equal(t, "passing", one.Thread.Links[0].CI)

	var list struct {
		Threads []db.Thread `json:"threads"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/"+p.ID+"/threads", nil, &list))
	require.Len(t, list.Threads, 1)
	require.Len(t, list.Threads[0].Links, 1)
}
