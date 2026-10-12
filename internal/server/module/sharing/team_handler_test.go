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

func TestHandler_RateLimit_CreateDefaultAndSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	h := NewHandler(manager.APIToken())
	router := gin.New()
	router.POST("/tokens", h.Create)
	router.GET("/tokens/:token_id", h.Get)
	router.PUT("/tokens/:token_id/rate-limit", h.SetRateLimit)

	// Omitted rpm_limit → default.
	created := sharingRequest(router, http.MethodPost, "/tokens", `{"display_name":"default"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var createResponse TokenCreateResponse
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}
	if createResponse.RPMLimit != db.DefaultSharingKeyRPMLimit {
		t.Fatalf("default rpm_limit = %d, want %d", createResponse.RPMLimit, db.DefaultSharingKeyRPMLimit)
	}

	// Explicit 0 → unlimited, and it must round-trip as 0 (not fall back to the default).
	unlimited := sharingRequest(router, http.MethodPost, "/tokens", `{"display_name":"unlimited","rpm_limit":0}`)
	var unlimitedResponse TokenCreateResponse
	if err := json.Unmarshal(unlimited.Body.Bytes(), &unlimitedResponse); err != nil {
		t.Fatal(err)
	}
	if unlimited.Code != http.StatusCreated || unlimitedResponse.RPMLimit != 0 {
		t.Fatalf("unlimited create = %d rpm_limit=%d", unlimited.Code, unlimitedResponse.RPMLimit)
	}

	// Out-of-range values are rejected by binding.
	if bad := sharingRequest(router, http.MethodPost, "/tokens", `{"display_name":"bad","rpm_limit":-1}`); bad.Code != http.StatusBadRequest {
		t.Fatalf("negative rpm_limit status = %d, want 400", bad.Code)
	}
	if huge := sharingRequest(router, http.MethodPost, "/tokens", `{"display_name":"huge","rpm_limit":1000001}`); huge.Code != http.StatusBadRequest {
		t.Fatalf("rpm_limit above max status = %d, want 400", huge.Code)
	}
	if huge := sharingRequest(router, http.MethodPut, "/tokens/"+createResponse.TokenID+"/rate-limit", `{"rpm_limit":9223372036854775807}`); huge.Code != http.StatusBadRequest {
		t.Fatalf("set rpm_limit above max status = %d, want 400", huge.Code)
	}

	// Set on an existing key and read back through GET.
	set := sharingRequest(router, http.MethodPut, "/tokens/"+createResponse.TokenID+"/rate-limit", `{"rpm_limit":12}`)
	if set.Code != http.StatusOK {
		t.Fatalf("set status = %d: %s", set.Code, set.Body.String())
	}
	var info APITokenInfo
	if err := json.Unmarshal(set.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.RPMLimit != 12 {
		t.Fatalf("set rpm_limit = %d, want 12", info.RPMLimit)
	}
	got := sharingRequest(router, http.MethodGet, "/tokens/"+createResponse.TokenID, "")
	if err := json.Unmarshal(got.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.RPMLimit != 12 {
		t.Fatalf("get rpm_limit = %d, want 12", info.RPMLimit)
	}

	// Missing body field and unknown key.
	if missing := sharingRequest(router, http.MethodPut, "/tokens/"+createResponse.TokenID+"/rate-limit", `{}`); missing.Code != http.StatusBadRequest {
		t.Fatalf("missing rpm_limit status = %d, want 400", missing.Code)
	}
	if notFound := sharingRequest(router, http.MethodPut, "/tokens/nope/rate-limit", `{"rpm_limit":5}`); notFound.Code != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404", notFound.Code)
	}
}
