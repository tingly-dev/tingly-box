package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/db"
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

	h := NewHandler(manager.Team())
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
	if info.ID == "" || info.Slug != "team1" || info.IsDefault || !info.Enabled {
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
	if updatedInfo.Name != "Research Lab" || updatedInfo.Slug != "team1" {
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
	if len(listed.Teams) != 1 || !listed.Teams[0].IsDefault {
		t.Fatalf("teams after delete = %+v", listed.Teams)
	}

	defaultDelete := performRequest(router, http.MethodDelete, "/teams/"+db.DefaultTeamID, "")
	if defaultDelete.Code != http.StatusConflict {
		t.Fatalf("default delete status = %d: %s", defaultDelete.Code, defaultDelete.Body.String())
	}
}
