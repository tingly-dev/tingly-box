package managedagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/swagger"
)

func newTestRouter(t *testing.T) (*gin.Engine, *Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	_, stores := managedagent.NewMemStores()
	svc := managedagent.NewService(managedagent.Config{Stores: stores})
	engine := gin.New()
	h := NewHandler(svc)
	h.ssePoll = 10 * time.Millisecond
	group := swagger.NewRouteManager(engine).NewGroup("api", "v1", "")
	RegisterRoutes(group, h)
	return engine, h
}

func do(t *testing.T, engine *gin.Engine, method, path, body string, out any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if out != nil && rec.Code < 300 && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec
}

// The whole local surface over HTTP: add a folder, start a task in it, read
// it back, steer it, archive it.
func TestSessionFlow(t *testing.T) {
	engine, _ := newTestRouter(t)
	dir := t.TempDir()

	var modes PermissionModeListResponse
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/permission-modes", "", &modes); rec.Code != 200 || len(modes.PermissionModes) == 0 {
		t.Fatalf("permission modes: %d %s", rec.Code, rec.Body)
	}

	var folder managedagent.Folder
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/folders", `{"path":"`+dir+`"}`, &folder); rec.Code != 201 {
		t.Fatalf("add folder: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/folders", `{"path":"relative"}`, nil); rec.Code != 400 {
		t.Fatalf("relative path: want 400, got %d", rec.Code)
	}
	var folders FolderListResponse
	do(t, engine, http.MethodGet, "/api/v1/agent/folders", "", &folders)
	if len(folders.Folders) != 1 || folders.Folders[0].Path != dir {
		t.Fatalf("folders = %+v", folders.Folders)
	}

	var detail SessionDetail
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"folder_id":"`+folder.ID+`","prompt":"add tests"}`, &detail); rec.Code != 201 {
		t.Fatalf("create session: %d %s", rec.Code, rec.Body)
	}
	if detail.Folder == nil || detail.Folder.Path != dir || detail.Session.Status != managedagent.SessionQueued {
		t.Fatalf("detail = %+v", detail)
	}
	id := detail.Session.ID

	// A folder takes several tasks at once, like several claude sessions in
	// one directory.
	var second SessionDetail
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"folder_id":"`+folder.ID+`","prompt":"another"}`, &second); rec.Code != 201 {
		t.Fatalf("second task in the same folder: %d %s", rec.Code, rec.Body)
	}
	// Removing the folder they work in is still refused while they run.
	if rec := do(t, engine, http.MethodDelete, "/api/v1/agent/folders/"+folder.ID, "", nil); rec.Code != 409 {
		t.Fatalf("removing a busy folder: want 409, got %d", rec.Code)
	}

	var list SessionListResponse
	do(t, engine, http.MethodGet, "/api/v1/agent/sessions?active=true", "", &list)
	if len(list.Sessions) != 2 || list.Sessions[0].Folder == nil || list.Sessions[0].Folder.Name != folder.Name {
		t.Fatalf("list = %+v", list.Sessions)
	}

	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", `{"text":"and docs"}`, nil); rec.Code != 202 {
		t.Fatalf("send message: %d %s", rec.Code, rec.Body)
	}
	var page EventListResponse
	do(t, engine, http.MethodGet, "/api/v1/agent/sessions/"+id+"/events?after=0", "", &page)
	if len(page.Events) != 2 || page.Events[0].Text != "add tests" || page.Events[1].Text != "and docs" {
		t.Fatalf("events = %+v", page.Events)
	}

	// A plain folder has no baseline, so the diff is empty rather than an error.
	var diff managedagent.Diff
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/sessions/"+id+"/diff", "", &diff); rec.Code != 200 || diff.ChangedFiles != 0 {
		t.Fatalf("diff: %d %+v", rec.Code, diff)
	}

	if rec := do(t, engine, http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", `{"permission_mode":"bypassPermissions"}`, &detail); rec.Code != 200 || detail.Session.PermissionMode != managedagent.PermissionBypassPermissions {
		t.Fatalf("set mode: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", `{"permission_mode":"yolo"}`, nil); rec.Code != 400 {
		t.Fatalf("bad mode: want 400, got %d", rec.Code)
	}

	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", "", &detail); rec.Code != 200 || detail.Session.Status != managedagent.SessionArchived {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+second.Session.ID+"/archive", "", nil)
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", `{"text":"too late"}`, nil); rec.Code != 409 {
		t.Fatalf("steer after archive: want 409, got %d", rec.Code)
	}
	// With the task ended, the folder can be withdrawn.
	if rec := do(t, engine, http.MethodDelete, "/api/v1/agent/folders/"+folder.ID, "", nil); rec.Code != 204 {
		t.Fatalf("remove folder: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/sessions/does-not-exist", "", nil); rec.Code != 404 {
		t.Fatalf("missing session: want 404, got %d", rec.Code)
	}
}

// Browsing is an allowlist: nothing is listable until a folder is handed
// over, and then only inside it.
func TestBrowseIsAnAllowlist(t *testing.T) {
	engine, _ := newTestRouter(t)
	dir := t.TempDir()

	var listing managedagent.DirListing
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs", "", &listing); rec.Code != 200 || len(listing.Entries) != 0 {
		t.Fatalf("empty allowlist: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path="+dir, "", nil); rec.Code != 403 {
		t.Fatalf("browse before adding: want 403, got %d", rec.Code)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path=nope", "", nil); rec.Code != 400 {
		t.Fatalf("relative path: want 400, got %d", rec.Code)
	}

	// Starting a task from a path is itself the grant.
	var detail SessionDetail
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"path":"`+dir+`","prompt":"go"}`, &detail); rec.Code != 201 {
		t.Fatalf("create from path: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path="+dir, "", &listing); rec.Code != 200 || listing.Path != dir {
		t.Fatalf("browse after adding: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path=/", "", nil); rec.Code != 403 {
		t.Fatalf("outside the allowlist: want 403, got %d", rec.Code)
	}
}

func TestEventsStream(t *testing.T) {
	engine, _ := newTestRouter(t)
	var detail SessionDetail
	do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"path":"`+t.TempDir()+`","prompt":"hi"}`, &detail)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/"+detail.Session.ID+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req.WithContext(ctx))
	if !strings.Contains(rec.Body.String(), "event:event") || !strings.Contains(rec.Body.String(), "hi") {
		t.Fatalf("stream body = %q", rec.Body.String())
	}
}
