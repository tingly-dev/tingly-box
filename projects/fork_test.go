package projects_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// callFile uploads one multipart "files" entry, the way the web does.
func callFile(t *testing.T, engine *gin.Engine, method, path, name, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("files", name)
	require.NoError(t, err)
	_, err = fw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestFork forks a project over the API and checks that the library and the
// settings came along while the conversation and threads did not. (What a
// populated project copies and keeps out is asserted in detail by the
// service's own tests.)
func TestFork(t *testing.T) {
	engine, _ := host(t)

	w := call(engine, "POST", "/api/v1/projects", `{"name":"Origin","goal":"Ship v1"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var src struct {
		ID           string `json:"id"`
		MainThreadID string `json:"main_thread_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &src))

	// A library file, so the fork has an asset to carry.
	upload := callFile(t, engine, "POST", fmt.Sprintf("/api/v1/projects/%s/library", src.ID), "map.txt", "the map")
	require.Equal(t, http.StatusOK, upload.Code, upload.Body.String())

	w = call(engine, "POST", fmt.Sprintf("/api/v1/projects/%s/fork", src.ID), `{"name":"Second attempt"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var fork struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Goal         string `json:"goal"`
		MainThreadID string `json:"main_thread_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fork))
	require.Equal(t, "Second attempt", fork.Name)
	require.Equal(t, "Ship v1", fork.Goal)
	require.NotEqual(t, src.ID, fork.ID)
	require.NotEqual(t, src.MainThreadID, fork.MainThreadID)

	// The fork's library holds the file; its threads are empty and its
	// coordinator is a fresh session.
	w = call(engine, "GET", fmt.Sprintf("/api/v1/projects/%s/library", fork.ID), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"path":"map.txt"`)
	w = call(engine, "GET", fmt.Sprintf("/api/v1/projects/%s", fork.ID), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail struct {
		Threads     []any          `json:"threads"`
		Coordinator map[string]any `json:"coordinator"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Empty(t, detail.Threads)
	require.NotEmpty(t, detail.Coordinator, "the fork has a coordinator to talk to")

	// The origin is untouched: its library still has the file.
	w = call(engine, "GET", fmt.Sprintf("/api/v1/projects/%s/library", src.ID), "")
	require.Contains(t, w.Body.String(), `"path":"map.txt"`)

	// The default name is "<source> (fork)".
	w = call(engine, "POST", fmt.Sprintf("/api/v1/projects/%s/fork", src.ID), `{}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"name":"Origin (fork)"`)

	// A working folder cannot be forked.
	w = call(engine, "POST", "/api/v1/projects", `{"type":"folder","path":"`+t.TempDir()+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var folder struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &folder))
	w = call(engine, "POST", fmt.Sprintf("/api/v1/projects/%s/fork", folder.ID), `{}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
