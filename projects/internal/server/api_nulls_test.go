package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// nullable lists the response fields that are null on purpose: a value that is
// absent, as opposed to a list that is empty.
var nullable = map[string]bool{"profile_override": true}

// nullsIn collects the paths of every JSON null in v that is not allowed.
func nullsIn(path string, v any, out *[]string) {
	switch x := v.(type) {
	case nil:
		if !nullable[path[strings.LastIndex(path, ".")+1:]] {
			*out = append(*out, path)
		}
	case map[string]any:
		for k, e := range x {
			nullsIn(path+"."+k, e, out)
		}
	case []any:
		for i, e := range x {
			nullsIn(fmt.Sprintf("%s[%d]", path, i), e, out)
		}
	}
}

// The web client calls .length, .map and .filter on the lists of a response
// without checking them, and a Go nil slice marshals to null: a project with no
// directories (repos), a thread with no links to refresh (results), a branch
// with no changes (files) each crashed a page that way. This reads every
// response of an empty workspace (a project, a folder, and a thread of each)
// and fails on any null that is not listed in nullable.
func TestResponsesOfAnEmptyWorkspaceCarryNoNull(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no gh: a refresh cannot reach the network
	store, c := newServer(t, quietAgents())
	var p, f db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	th := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle}
	require.NoError(t, store.CreateThread(th))
	fth := &db.Thread{ProjectID: f.ID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle}
	require.NoError(t, store.CreateThread(fth))
	require.NoError(t, store.AddThreadEvent(&db.ThreadEvent{ThreadID: th.ID, Kind: db.EvUser, Content: "hi"}))

	read := func(method, path string) {
		req, err := http.NewRequest(method, c.base+path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		require.Less(t, resp.StatusCode, 400, "%s %s: %s", method, path, body)
		var v any
		require.NoError(t, json.Unmarshal(body, &v), "%s %s", method, path)
		var found []string
		nullsIn("", v, &found)
		sort.Strings(found)
		assert.Empty(t, found, "%s %s", method, path)
	}
	for _, path := range []string{
		"/projects", "/projects?archived=true", "/projects/channel-bindings", "/projects/runtime", "/projects/models",
		"/projects/" + p.ID, "/projects/" + f.ID, "/projects/" + p.ID + "/threads", "/projects/" + f.ID + "/threads",
		"/projects/" + p.ID + "/memory", "/projects/" + p.ID + "/library", "/projects/" + p.ID + "/library/tree", "/projects/" + p.ID + "/usage",
		"/threads/permission-modes", "/conversations", "/settings", "/chats",
		"/threads/" + th.ID, "/threads/" + fth.ID, "/threads/" + p.MainThreadID,
		"/threads/" + th.ID + "/events", "/threads/" + th.ID + "/requests", "/threads/" + th.ID + "/subagents", "/threads/" + th.ID + "/cleanup",
	} {
		read("GET", path)
	}
	read("POST", "/projects/"+p.ID+"/links/refresh")
	read("POST", "/threads/"+th.ID+"/links/refresh")
}
