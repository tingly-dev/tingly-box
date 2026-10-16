package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	projectsmodule "github.com/tingly-dev/tingly-box/internal/server/module/projects"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/projects/llm"
	"github.com/tingly-dev/tingly-box/remote/session"
	"github.com/tingly-dev/tingly-box/swagger"
)

type importHost struct{}

func (importHost) ClientFor(context.Context, string) (llm.Client, string, error) {
	return nil, "", nil
}
func (importHost) Models(context.Context) ([]llm.Model, error)  { return nil, nil }
func (importHost) ThreadEnv(context.Context) ([]string, string) { return nil, "" }

// Desk's sessions (ChatID "web") come over as working-folder sessions;
// IM @cc sessions in the same store stay where they are.
func TestImportDeskSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sm, err := db.NewStoreManager(t.TempDir())
	require.NoError(t, err)
	store := sm.RemoteSessions()
	at := time.Now().Add(-time.Hour)
	folder := t.TempDir()
	require.NoError(t, store.Set("cc-web", &session.Session{
		ID: "cc-web", ChatID: deskChatID, Project: folder, Status: session.StatusCompleted,
		Request: "fix the build", Profile: "p1", CreatedAt: at, LastActivity: at,
	}))
	require.NoError(t, store.AppendMessage("cc-web", session.Message{Role: "user", Content: "fix the build", Timestamp: at}))
	require.NoError(t, store.AppendMessage("cc-web", session.Message{Role: "assistant", Content: "Fixed.", Timestamp: at}))
	require.NoError(t, store.Set("cc-im", &session.Session{ID: "cc-im", ChatID: "tg:1", Project: folder, Status: session.StatusCompleted}))

	app, err := projectsapp.New(projectsapp.Options{Host: importHost{}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })
	importDeskSessions(sm, app)

	engine := gin.New()
	module.Mount(&module.Routes{Manager: swagger.NewRouteManager(engine)}, projectsmodule.NewHandler(app, func() bool { return true }, func() {}))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects/sessions", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Sessions []struct {
			Title, Worktree, Profile, Status string
			SessionID                        string `json:"session_id"`
		} `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Sessions, 1)
	s := got.Sessions[0]
	assert.Equal(t, "fix the build", s.Title)
	assert.Equal(t, folder, s.Worktree)
	assert.Equal(t, "p1", s.Profile)
	assert.Equal(t, "cc-web", s.SessionID)

	// The originals stay in the remote store.
	kept, _ := store.ListByChat(deskChatID)
	assert.Len(t, kept, 1)
}
