package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestDecisionsEndpoint_ScenariosPassTransportGate pins that
// /tingly/team[/v1]/decisions (and the openai and dedicated decisions
// scenarios) clears the scenario transport gate (the team
// descriptor declares TransportOpenAI) for both the default team and an
// isolated team scope ("team:<id>" derived by teamScopeMiddleware). With no
// rules configured the request must reach rule resolution ("not configured"),
// never the transport rejection.
func TestDecisionsEndpoint_ScenariosPassTransportGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)

	body := `{"model":"decision-model","options":["a","b"]}`
	tests := []struct{ name, path, teamID, wantErr string }{
		{name: "default team /v1", path: "/tingly/team/v1/decisions", wantErr: "not configured"},
		{name: "default team bare", path: "/tingly/team/decisions", wantErr: "not configured"},
		{name: "isolated team /v1", path: "/tingly/team/v1/decisions", teamID: "team-a", wantErr: "not configured"},
		{name: "dedicated decisions scenario", path: "/tingly/decisions/v1/decisions", wantErr: "not configured"},
		{name: "openai scenario", path: "/tingly/openai/v1/decisions", wantErr: "not configured"},
		// Control: an anthropic-only scenario still fails closed.
		{name: "rejected for anthropic-only scenario", path: "/tingly/claude_code/v1/decisions", wantErr: "does not support"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.teamID != "" {
				req.Header.Set("X-Test-Team-ID", tt.teamID)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Contains(t, w.Body.String(), tt.wantErr)
		})
	}
}
