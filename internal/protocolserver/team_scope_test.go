package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestTeamScopeMiddleware_DerivesRoutingScopeFromAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ph := &ProtocolHandler{}
	router := gin.New()
	router.POST("/tingly/:scenario/messages",
		func(c *gin.Context) {
			if teamID := c.GetHeader("X-Test-Team-ID"); teamID != "" {
				c.Set(constant.CtxKeyTeamID, teamID)
			}
		},
		ph.teamScopeMiddleware,
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"scenario": c.Param("scenario"),
				"team_id":  c.GetString(constant.CtxKeyTeamID),
				"path":     c.Request.URL.Path,
			})
		},
	)

	tests := []struct {
		name         string
		path         string
		teamID       string
		wantScenario string
		wantTeamID   string
	}{
		{name: "global auth defaults legacy team", path: "/tingly/team/messages", wantScenario: "team", wantTeamID: db.DefaultTeamID},
		{name: "default sharing team keeps legacy scope", path: "/tingly/team/messages", teamID: db.DefaultTeamID, wantScenario: "team", wantTeamID: db.DefaultTeamID},
		{name: "other sharing team gets isolated scope", path: "/tingly/team/messages", teamID: "team-a", wantScenario: "team:team-a", wantTeamID: "team-a"},
		{name: "non-team scenario is unchanged", path: "/tingly/openai/messages", teamID: "team-a", wantScenario: "openai", wantTeamID: "team-a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			if tt.teamID != "" {
				req.Header.Set("X-Test-Team-ID", tt.teamID)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, want := range []string{`"scenario":"` + tt.wantScenario + `"`, `"team_id":"` + tt.wantTeamID + `"`, `"path":"` + tt.path + `"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("body %s does not contain %s", body, want)
				}
			}
		})
	}
}

func TestUsageRecordCarriesAuthenticatedTeam(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set(constant.CtxKeyUserID, "user-a")
	c.Set(constant.CtxKeyTeamID, "team-a")
	provider := &typ.Provider{UUID: "provider-a", Name: "Provider A"}
	record := (&ProtocolHandler{}).recordDetailedUsage(c, nil, provider, "model", "request-model", "team", 1, 2, false, "success", "", 3)
	if record.TeamID != "team-a" {
		t.Fatalf("TeamID = %q, want team-a", record.TeamID)
	}
}

func TestTeamScopeMiddleware_SelectsOnlyAuthorizedTeamRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Rules: []typ.Rule{
		{UUID: "default-rule", Scenario: typ.ScenarioTeam, RequestModel: "shared-model", Active: true},
		{UUID: "team-a-rule", Scenario: typ.RuleScenario("team:team-a"), RequestModel: "shared-model", Active: true},
		{UUID: "team-b-rule", Scenario: typ.RuleScenario("team:team-b"), RequestModel: "shared-model", Active: true},
	}}
	ph := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: cfg}}
	router := gin.New()
	router.POST("/tingly/:scenario/messages",
		func(c *gin.Context) { c.Set(constant.CtxKeyTeamID, c.GetHeader("X-Test-Team-ID")) },
		ph.teamScopeMiddleware,
		func(c *gin.Context) {
			rule, err := ph.determineRuleWithScenario(c, typ.RuleScenario(c.Param("scenario")), "shared-model")
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"rule": rule.UUID})
		},
	)

	for _, tt := range []struct {
		teamID   string
		wantRule string
	}{
		{teamID: db.DefaultTeamID, wantRule: "default-rule"},
		{teamID: "team-a", wantRule: "team-a-rule"},
		{teamID: "team-b", wantRule: "team-b-rule"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/tingly/team/messages", nil)
		req.Header.Set("X-Test-Team-ID", tt.teamID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"rule":"`+tt.wantRule+`"`) {
			t.Fatalf("team %s got status %d body %s; want %s", tt.teamID, w.Code, w.Body.String(), tt.wantRule)
		}
	}
}
