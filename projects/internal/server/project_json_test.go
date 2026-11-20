package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The web client calls .length and .map on repos and on every list in a
// response, so an empty one must reach it as [], never null: a Go nil slice
// marshals to null, and a project with no repositories has a nil one.
func TestEmptyListsAreArraysNotNull(t *testing.T) {
	_, c := newServer(t, quietAgents())

	var created map[string]any
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "No repos"}, &created))
	assert.Equal(t, []any{}, created["repos"], "create")
	id := created["id"].(string)

	var list struct {
		Projects []map[string]any `json:"projects"`
	}
	require.Equal(t, 200, c.do("GET", "/projects", nil, &list))
	require.Len(t, list.Projects, 1)
	assert.Equal(t, []any{}, list.Projects[0]["repos"], "list: the project card reads repos.length")

	var detail struct {
		Project map[string]any `json:"project"`
		Threads []any          `json:"threads"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/"+id, nil, &detail))
	assert.Equal(t, []any{}, detail.Project["repos"], "detail")

	var patched map[string]any
	require.Equal(t, 200, c.do("PATCH", "/projects/"+id, map[string]any{"goal": "g"}, &patched))
	assert.Equal(t, []any{}, patched["repos"], "patch")

	var threads struct {
		Threads []any `json:"threads"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/"+id+"/threads", nil, &threads))
	assert.NotNil(t, threads.Threads, "a project's thread list is [] while it has no thread")
}
