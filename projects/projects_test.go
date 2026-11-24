package projects_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/swagger"
)

type fakeHost struct{}

func (fakeHost) Models(context.Context) ([]projects.Model, error) {
	return []projects.Model{{ID: "host-model"}}, nil
}

func (fakeHost) ThreadEnv(context.Context) ([]string, string) {
	return []string{"ANTHROPIC_BASE_URL=http://gateway.test/anthropic"}, ""
}

// host mounts Projects the way an embedding server does: on its own
// /api/v1 swagger group.
func host(t *testing.T) (*gin.Engine, *swagger.RouteManager) {
	t.Helper()
	app, err := projects.New(projects.Options{DataDir: t.TempDir(), Host: fakeHost{}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app.Start(ctx)
	app.Start(ctx) // a second call is a no-op

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	m := swagger.NewRouteManager(engine)
	app.Register(m.NewGroup("api", "v1", ""))
	return engine, m
}

func call(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestEmbedded(t *testing.T) {
	engine, _ := host(t)

	w := call(engine, "GET", "/api/v1/projects/models", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "host-model")

	w = call(engine, "GET", "/api/v1/projects/runtime", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "http://gateway.test/anthropic")

	w = call(engine, "POST", "/api/v1/projects", `{"name":"Embedded"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var p struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	w = call(engine, "GET", "/api/v1/projects/"+p.ID, "")
	assert.Equal(t, http.StatusOK, w.Code)

	// What the host owns itself is not served by Projects.
	for _, path := range []string{"/api/v1/settings", "/api/v1/models", "/api/v1/conversations", "/api/v1/health"} {
		assert.Equal(t, http.StatusNotFound, call(engine, "GET", path, "").Code, path)
	}
}

func TestEmbeddedOpenAPI(t *testing.T) {
	_, m := host(t)
	spec, err := m.GenerateOpenAPI(swagger.VersionV3)
	require.NoError(t, err)
	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(spec), &doc))
	require.NotEmpty(t, doc.Paths)
	for path := range doc.Paths {
		assert.True(t, strings.HasPrefix(path, "/api/v1/projects") || strings.HasPrefix(path, "/api/v1/threads"), "path outside /projects and /threads: %s", path)
	}
}
