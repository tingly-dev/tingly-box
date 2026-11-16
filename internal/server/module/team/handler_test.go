package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func performRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestHandler_TeamLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	h := NewHandler(manager.Team(), nil)
	router := gin.New()
	router.GET("/teams", h.List)
	router.POST("/teams", h.Create)
	router.PUT("/teams/:team_id", h.Update)
	router.PUT("/teams/:team_id/disable", h.Disable)
	router.DELETE("/teams/:team_id", h.Delete)

	created := performRequest(router, http.MethodPost, "/teams", `{"name":"Research"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var info TeamInfo
	if err := json.Unmarshal(created.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.ID == "" || info.Slug != "t1" || info.IsDefault || !info.Enabled {
		t.Fatalf("unexpected created team: %+v", info)
	}

	duplicate := performRequest(router, http.MethodPost, "/teams", `{"name":"Research"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d: %s", duplicate.Code, duplicate.Body.String())
	}

	updated := performRequest(router, http.MethodPut, "/teams/"+info.ID, `{"name":"Research Lab"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updated.Code, updated.Body.String())
	}
	var updatedInfo TeamInfo
	if err := json.Unmarshal(updated.Body.Bytes(), &updatedInfo); err != nil {
		t.Fatal(err)
	}
	if updatedInfo.Name != "Research Lab" || updatedInfo.Slug != "t1" {
		t.Fatalf("unexpected updated team: %+v", updatedInfo)
	}

	disabled := performRequest(router, http.MethodPut, "/teams/"+info.ID+"/disable", "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status = %d: %s", disabled.Code, disabled.Body.String())
	}

	deleted := performRequest(router, http.MethodDelete, "/teams/"+info.ID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", deleted.Code, deleted.Body.String())
	}

	list := performRequest(router, http.MethodGet, "/teams", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	var listed ListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Teams) != 1 || !listed.Teams[0].IsDefault || listed.Teams[0].Name != db.DefaultTeamName {
		t.Fatalf("teams after delete = %+v", listed.Teams)
	}

	defaultDelete := performRequest(router, http.MethodDelete, "/teams/"+db.DefaultTeamID, "")
	if defaultDelete.Code != http.StatusConflict {
		t.Fatalf("default delete status = %d: %s", defaultDelete.Code, defaultDelete.Body.String())
	}
}

func TestHandler_DuplicateAndDeleteCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg.StoreManager().Team(), cfg)
	router := gin.New()
	router.POST("/teams", h.Create)
	router.POST("/teams/:team_id/duplicate", h.Duplicate)
	router.DELETE("/teams/:team_id", h.Delete)

	created := performRequest(router, http.MethodPost, "/teams", `{"name":"Research"}`)
	var src TeamInfo
	if err := json.Unmarshal(created.Body.Bytes(), &src); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	srcScope := db.TeamScenario(src.ID)
	if err := cfg.AddRule(typ.Rule{Scenario: srcScope, RequestModel: "m", Active: true}); err != nil {
		t.Fatal(err)
	}
	routingOf := func(scope typ.RuleScenario) int {
		n := 0
		for _, r := range cfg.GetRequestConfigs() {
			if r.Scenario == scope {
				n++
			}
		}
		return n
	}

	// An empty body is fine: the name is picked.
	dup := performRequest(router, http.MethodPost, "/teams/"+src.ID+"/duplicate", "")
	if dup.Code != http.StatusCreated {
		t.Fatalf("duplicate status = %d: %s", dup.Code, dup.Body.String())
	}
	var copied TeamInfo
	if err := json.Unmarshal(dup.Body.Bytes(), &copied); err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Research copy" || copied.ID == src.ID || copied.IsDefault {
		t.Fatalf("unexpected copy: %+v", copied)
	}
	if routingOf(db.TeamScenario(copied.ID)) != 1 {
		t.Fatalf("the copy has no routing")
	}

	if got := performRequest(router, http.MethodPost, "/teams/"+src.ID+"/duplicate", `{"name":"Research"}`).Code; got != http.StatusConflict {
		t.Fatalf("taken name status = %d", got)
	}
	if got := performRequest(router, http.MethodPost, "/teams/nope/duplicate", `{}`).Code; got != http.StatusNotFound {
		t.Fatalf("unknown source status = %d", got)
	}

	// Deleting a team takes its routing with it; the source's stays.
	if got := performRequest(router, http.MethodDelete, "/teams/"+copied.ID, "").Code; got != http.StatusNoContent {
		t.Fatalf("delete status = %d", got)
	}
	if routingOf(db.TeamScenario(copied.ID)) != 0 || routingOf(srcScope) != 1 {
		t.Fatalf("routing after delete: copy=%d source=%d", routingOf(db.TeamScenario(copied.ID)), routingOf(srcScope))
	}
}
