package sharing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/db"
)

func sharingRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestHandler_CreateListAndMoveTeamToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	teamA, err := manager.Team().Create("Team A")
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := manager.Team().Create("Team B")
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(manager.APIToken())
	router := gin.New()
	router.POST("/tokens", h.Create)
	router.GET("/tokens", h.List)
	router.PUT("/tokens/:token_id/team", h.MoveToTeam)

	created := sharingRequest(router, http.MethodPost, "/tokens", `{"display_name":"CI","team_id":"`+teamA.ID+`"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var createResponse TokenCreateResponse
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}
	if createResponse.TeamID != teamA.ID {
		t.Fatalf("created team = %q, want %q", createResponse.TeamID, teamA.ID)
	}

	listA := sharingRequest(router, http.MethodGet, "/tokens?team_id="+teamA.ID, "")
	var listResponse TokenListResponse
	if err := json.Unmarshal(listA.Body.Bytes(), &listResponse); err != nil {
		t.Fatal(err)
	}
	if listA.Code != http.StatusOK || listResponse.Total != 1 || listResponse.Tokens[0].TeamID != teamA.ID {
		t.Fatalf("team A list = %d %+v", listA.Code, listResponse)
	}

	moved := sharingRequest(router, http.MethodPut, "/tokens/"+createResponse.TokenID+"/team", `{"team_id":"`+teamB.ID+`"}`)
	if moved.Code != http.StatusOK {
		t.Fatalf("move status = %d: %s", moved.Code, moved.Body.String())
	}
	var movedInfo APITokenInfo
	if err := json.Unmarshal(moved.Body.Bytes(), &movedInfo); err != nil {
		t.Fatal(err)
	}
	if movedInfo.TeamID != teamB.ID {
		t.Fatalf("moved team = %q, want %q", movedInfo.TeamID, teamB.ID)
	}

	listA = sharingRequest(router, http.MethodGet, "/tokens?team_id="+teamA.ID, "")
	if err := json.Unmarshal(listA.Body.Bytes(), &listResponse); err != nil {
		t.Fatal(err)
	}
	if listResponse.Total != 0 {
		t.Fatalf("old team total = %d, want 0", listResponse.Total)
	}

	if err := manager.Team().SetEnabled(teamA.ID, false); err != nil {
		t.Fatal(err)
	}
	rejected := sharingRequest(router, http.MethodPut, "/tokens/"+createResponse.TokenID+"/team", `{"team_id":"`+teamA.ID+`"}`)
	if rejected.Code != http.StatusConflict {
		t.Fatalf("move to disabled team status = %d: %s", rejected.Code, rejected.Body.String())
	}
}
