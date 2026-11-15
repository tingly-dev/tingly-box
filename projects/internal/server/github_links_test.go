package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
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

func TestRefreshLinksOverTheAPI(t *testing.T) {
	// No gh on PATH: a refresh that reaches GitHub fails the way a machine
	// without gh does, and nothing in this test can reach the network.
	t.Setenv("PATH", t.TempDir())
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	th := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle}
	require.NoError(t, store.CreateThread(th))
	_, err := store.UpsertThreadLink(th.ID, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: 12,
		URL: "https://github.com/acme/widgets/pull/12", Title: "Add a", Role: db.LinkOpened, State: db.LinkStateOpen, CI: "passing"})
	require.NoError(t, err)

	var out rt.RefreshLinksResponse
	require.Equal(t, 200, c.do("POST", "/threads/"+th.ID+"/links/refresh", map[string]any{"repo": "acme/widgets", "kind": "pr", "number": 12}, &out))
	require.Len(t, out.Results, 1)
	assert.Equal(t, rt.RefreshFailed, out.Results[0].Status)
	assert.Contains(t, out.Results[0].Error, "GitHub is not available", "the cause is said, not hidden")
	got, err := store.FindThreadLink(th.ID, "acme/widgets", 12)
	require.NoError(t, err)
	assert.Equal(t, "passing", got.CI, "the last good reading stays")
	assert.Contains(t, got.LastError, "GitHub is not available")
	assert.Zero(t, got.Failures, "a person's refresh does not feed the back-off")

	// Within the cooldown nothing is asked of GitHub.
	require.Equal(t, 200, c.do("POST", "/threads/"+th.ID+"/links/refresh", nil, &out), "an empty body means all of the thread's links")
	require.Len(t, out.Results, 1)
	assert.Equal(t, rt.RefreshRecent, out.Results[0].Status)

	assert.Equal(t, 404, c.do("POST", "/threads/"+th.ID+"/links/refresh", map[string]any{"repo": "acme/widgets", "number": 99}, nil))
	assert.Equal(t, 404, c.do("POST", "/threads/nope/links/refresh", nil, nil))
	assert.Equal(t, 400, c.do("POST", "/threads/"+th.ID+"/links/refresh", map[string]any{"number": 12}, nil), "repo and number go together")
	assert.Equal(t, 400, c.do("POST", "/threads/"+th.ID+"/links/refresh", map[string]any{"repo": "acme/widgets", "kind": 3, "number": 12}, nil))

	require.Equal(t, 200, c.do("POST", "/projects/"+p.ID+"/links/refresh", nil, &out))
	assert.Equal(t, 404, c.do("POST", "/projects/nope/links/refresh", nil, nil))
}
