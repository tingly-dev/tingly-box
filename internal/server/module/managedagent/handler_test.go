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
	svc := managedagent.NewService(managedagent.Config{Stores: stores, WorkspacesDir: t.TempDir()})
	if err := svc.EnsureDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	h := NewHandler(svc)
	h.ssePoll = 10 * time.Millisecond
	group := swagger.NewRouteManager(engine).NewGroup("api", "v1", "")
	RegisterRoutes(group, h)
	return engine, h
}

func do(t *testing.T, engine *gin.Engine, method, path, body string, out any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
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

func TestSessionFlow(t *testing.T) {
	engine, _ := newTestRouter(t)

	var envs EnvironmentListResponse
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/environments", "", &envs); rec.Code != 200 {
		t.Fatalf("list environments: %d %s", rec.Code, rec.Body)
	}
	if len(envs.Environments) != 1 || !envs.Environments[0].IsDefault || len(envs.SupportedRuntimes) != 1 || len(envs.PermissionModes) == 0 {
		t.Fatalf("unexpected environments: %+v", envs)
	}

	// Docker is in the schema but rejected with a 400 that says so.
	rec := do(t, engine, http.MethodPost, "/api/v1/agent/environments", `{"name":"box","runtime":"docker","image":"x"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "not available yet") {
		t.Fatalf("docker env: %d %s", rec.Code, rec.Body)
	}

	var src managedagent.Source
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sources", `{"url":"https://github.com/org/repo.git"}`, &src); rec.Code != 201 {
		t.Fatalf("create source: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sources", `{"url":"nope"}`, nil); rec.Code != 400 {
		t.Fatalf("invalid source: %d", rec.Code)
	}

	var detail SessionDetail
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"source_id":"`+src.ID+`","prompt":"Add tests"}`, &detail); rec.Code != 201 {
		t.Fatalf("create session: %d %s", rec.Code, rec.Body)
	}
	if detail.Session.Status != managedagent.SessionQueued || detail.Workspace == nil || detail.Workspace.SourceID != src.ID {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	id := detail.Session.ID

	if rec := do(t, engine, http.MethodDelete, "/api/v1/agent/sources/"+src.ID, "", nil); rec.Code != 409 {
		t.Fatalf("delete source with live workspace: %d", rec.Code)
	}
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/messages", `{"text":"and docs"}`, nil); rec.Code != 202 {
		t.Fatalf("send message: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/respond", `{"request_id":"r"}`, nil); rec.Code != 409 {
		t.Fatalf("respond while queued: %d %s", rec.Code, rec.Body)
	}

	var page EventListResponse
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/sessions/"+id+"/events?after=1", "", &page); rec.Code != 200 {
		t.Fatalf("events: %d %s", rec.Code, rec.Body)
	}
	if len(page.Events) != 1 || page.Events[0].Text != "and docs" || page.Next != 2 {
		t.Fatalf("events page: %+v", page)
	}

	if rec := do(t, engine, http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", `{"permission_mode":"auto"}`, &detail); rec.Code != 200 || detail.Session.PermissionMode != managedagent.PermissionAuto {
		t.Fatalf("set permission mode: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodPut, "/api/v1/agent/sessions/"+id+"/permission-mode", `{"permission_mode":"yolo"}`, nil); rec.Code != 400 {
		t.Fatalf("bad permission mode: %d", rec.Code)
	}

	var list SessionListResponse
	do(t, engine, http.MethodGet, "/api/v1/agent/sessions?active=true", "", &list)
	if len(list.Sessions) != 1 || list.Sessions[0].Source == nil || list.Sessions[0].Source.ID != src.ID || list.Sessions[0].Branch == "" {
		t.Fatalf("active list: %+v", list)
	}
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions/"+id+"/archive", "", &detail); rec.Code != 200 || detail.Session.Status != managedagent.SessionArchived {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	do(t, engine, http.MethodGet, "/api/v1/agent/sessions?active=true", "", &list)
	if len(list.Sessions) != 0 {
		t.Fatalf("active list after archive: %+v", list)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/sessions/missing", "", nil); rec.Code != 404 {
		t.Fatalf("missing session: %d", rec.Code)
	}
}

func TestLocalFolderEndpoints(t *testing.T) {
	engine, h := newTestRouter(t)
	dir := t.TempDir()
	h.WithRecentProjects(func(context.Context) ([]string, error) { return []string{dir}, nil })

	var listing managedagent.DirListing
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path="+dir, "", &listing); rec.Code != 200 || listing.Path != dir {
		t.Fatalf("browse: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/dirs?path=nope", "", nil); rec.Code != 400 {
		t.Fatalf("browse relative: %d", rec.Code)
	}
	var recent RecentFoldersResponse
	if rec := do(t, engine, http.MethodGet, "/api/v1/agent/fs/recent", "", &recent); rec.Code != 200 || len(recent.Folders) != 1 || recent.Folders[0].Source != "claude_code" {
		t.Fatalf("recent: %d %s", rec.Code, rec.Body)
	}
	// Start a task straight from a folder: no source step in between.
	var detail SessionDetail
	if rec := do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"local_path":"`+dir+`","prompt":"go"}`, &detail); rec.Code != 201 {
		t.Fatalf("create from local_path: %d %s", rec.Code, rec.Body)
	}
	if detail.Workspace == nil || detail.Workspace.Path != dir || detail.Workspace.Branch != "" {
		t.Fatalf("in-place workspace expected: %+v", detail.Workspace)
	}
	do(t, engine, http.MethodGet, "/api/v1/agent/fs/recent", "", &recent)
	if len(recent.Folders) != 1 || recent.Folders[0].Source != "tasks" {
		t.Fatalf("recent after use should be the task folder once: %+v", recent.Folders)
	}
}

func TestEventsStream(t *testing.T) {
	engine, _ := newTestRouter(t)
	var src managedagent.Source
	do(t, engine, http.MethodPost, "/api/v1/agent/sources", `{"url":"https://github.com/org/repo.git"}`, &src)
	var detail SessionDetail
	do(t, engine, http.MethodPost, "/api/v1/agent/sessions", `{"source_id":"`+src.ID+`","prompt":"hi"}`, &detail)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/"+detail.Session.ID+"/events", nil).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event:event") || !strings.Contains(body, `"text":"hi"`) || !strings.Contains(body, "event:ping") {
		t.Fatalf("stream body:\n%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/missing/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("stream on missing session: %d", rec.Code)
	}
}
