package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestChannelBindingRoutes(t *testing.T) {
	_, c := newServer(t, nil)
	var a, b db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "A"}, &a))
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "B"}, &b))

	type list struct {
		Bindings []db.ChannelBinding `json:"bindings"`
	}
	var l list
	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings", nil, &l))
	assert.Empty(t, l.Bindings)

	// A project id is resolved to its main thread; the response carries both.
	var bound db.ChannelBinding
	require.Equal(t, 200, c.do("PUT", "/projects/channel-bindings", map[string]any{
		"platform": "telegram", "chat_id": "42", "project_id": a.ID, "chat_title": "Me", "bot_uuid": "bot-2",
	}, &bound))
	assert.NotEmpty(t, bound.ID)
	assert.Equal(t, a.MainThreadID, bound.ThreadID)
	assert.Equal(t, a.ID, bound.ProjectID)
	assert.Equal(t, "Me", bound.ChatTitle)
	assert.Equal(t, "bot-2", bound.BotUUID)

	// Binding the chat again moves it rather than adding a second row; a
	// thread id binds that thread.
	var moved db.ChannelBinding
	require.Equal(t, 200, c.do("PUT", "/projects/channel-bindings", map[string]any{
		"platform": "telegram", "chat_id": "42", "thread_id": b.MainThreadID,
	}, &moved))
	assert.Equal(t, bound.ID, moved.ID)
	assert.Equal(t, b.MainThreadID, moved.ThreadID)
	assert.Equal(t, b.ID, moved.ProjectID)
	assert.Equal(t, "bot-2", moved.BotUUID, "legacy callers retain the selected bot")

	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings", nil, &l))
	assert.Len(t, l.Bindings, 1)
	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings?project_id="+a.ID, nil, &l))
	assert.Empty(t, l.Bindings)
	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings?project_id="+b.ID, nil, &l))
	require.Len(t, l.Bindings, 1)
	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings?thread_id="+b.MainThreadID, nil, &l))
	require.Len(t, l.Bindings, 1)
	assert.Equal(t, b.ID, l.Bindings[0].ProjectID)

	// The static route does not shadow the project routes around it.
	var detail struct {
		Project db.Project `json:"project"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/"+b.ID, nil, &detail))
	assert.Equal(t, "B", detail.Project.Name)

	require.Equal(t, 400, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1"}, nil))
	require.Equal(t, 400, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1", "project_id": a.ID, "thread_id": a.MainThreadID}, nil))
	require.Equal(t, 404, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1", "project_id": "nope"}, nil))
	require.Equal(t, 404, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1", "thread_id": "nope"}, nil))

	// A folder has no main thread: bind one of its threads instead.
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	require.Equal(t, 400, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1", "project_id": f.ID}, nil))

	require.Equal(t, 200, c.do("DELETE", "/projects/channel-bindings/"+bound.ID, nil, nil))
	require.Equal(t, 404, c.do("DELETE", "/projects/channel-bindings/"+bound.ID, nil, nil))
	require.Equal(t, 200, c.do("GET", "/projects/channel-bindings", nil, &l))
	assert.Empty(t, l.Bindings)
}

// The standalone app has no IM, so a bound project still works: binding is
// just a row nothing sends to.
func TestPostingToABoundProjectStillWorksWithoutAnIMHost(t *testing.T) {
	_, c := newServer(t, nil)
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	require.Equal(t, 200, c.do("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "1", "project_id": p.ID}, nil))
	require.Equal(t, 200, c.do("POST", "/threads/"+p.MainThreadID+"/messages", map[string]any{"content": "hi"}, nil))
	var evs struct {
		Events []db.ThreadEvent `json:"events"`
	}
	require.Equal(t, 200, c.do("GET", "/threads/"+p.MainThreadID+"/events", nil, &evs))
	require.NotEmpty(t, evs.Events)
	assert.Equal(t, db.EvUser, evs.Events[0].Kind)
	assert.Empty(t, evs.Events[0].Source, "a web message has no source")
}
