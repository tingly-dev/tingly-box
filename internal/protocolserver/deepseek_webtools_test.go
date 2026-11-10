package protocolserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/constant"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
)

func TestDeepSeekWebGatewayPassthroughAndAuth(t *testing.T) {
	for _, op := range []string{"search", "fetch"} {
		t.Run(op, func(t *testing.T) {
			calls, records := 0, 0
			payload := `{"query":"test","count":3,"extra":"preserve"}`
			response := `{"results":[{"title":"Page","url":"https://example.com","summary":"summary","extra":123}],"extra":"preserve"}`
			if op == "fetch" {
				payload = `{"url":"https://example.com","extra":"preserve"}`
				response = `{"title":"Page","content":"markdown","extra":"preserve"}`
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
				require.Equal(t, "/v1/tools/"+op, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, payload, string(body))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, response)
			}))
			defer upstream.Close()
			service := &deepseek.Service{Resolve: func() *typ.Provider { return &typ.Provider{APIBase: upstream.URL, Token: "upstream-key"} }, Record: func(ctx context.Context, _ *typ.Provider, operation string, billable bool, _ time.Duration, err error) {
				records++
				require.Equal(t, op, operation)
				require.True(t, billable)
				require.NoError(t, err)
				require.Equal(t, "caller", coretool.GetUsageIdentity(ctx).UserID)
			}}
			router := gin.New()
			NewHandler(ProtocolHandlerDeps{DeepSeekWebtools: service}).RegisterRoutes(router, func(c *gin.Context) {
				if c.GetHeader("Authorization") != "Bearer gateway-key" {
					c.AbortWithStatus(401)
					return
				}
				c.Set(constant.CtxKeyUserID, "caller")
				c.Next()
			})
			for _, authorized := range []bool{false, true} {
				req := httptest.NewRequest(http.MethodPost, "/v1/tools/"+op, strings.NewReader(payload))
				if authorized {
					req.Header.Set("Authorization", "Bearer gateway-key")
				}
				out := httptest.NewRecorder()
				router.ServeHTTP(out, req)
				if !authorized {
					require.Equal(t, 401, out.Code)
					require.Equal(t, 0, calls)
				} else {
					require.Equal(t, 200, out.Code)
					require.Equal(t, response, out.Body.String())
				}
			}
			require.Equal(t, 1, calls)
			require.Equal(t, 1, records)
		})
	}
}

func TestDeepSeekWebGatewayErrorEnvelopesAndTeamScope(t *testing.T) {
	response := `{"error":{"message":"insufficient balance","type":"insufficient_quota","code":"insufficient_quota","param":null}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(402); fmt.Fprint(w, response) }))
	defer upstream.Close()
	service := &deepseek.Service{Resolve: func() *typ.Provider { return &typ.Provider{APIBase: upstream.URL, Token: "secret"} }}
	for _, tc := range []struct {
		service *deepseek.Service
		team    string
		status  int
	}{{service, "", 402}, {nil, "", 503}, {service, "team-id", 403}} {
		router := gin.New()
		router.POST("/v1/tools/search", func(c *gin.Context) { c.Set(constant.CtxKeyTeamID, tc.team) }, deepSeekWebHandler(tc.service, "search"))
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest("POST", "/v1/tools/search", strings.NewReader(`{"query":"test"}`)))
		require.Equal(t, tc.status, out.Code)
		if tc.status == 402 {
			require.Equal(t, response, out.Body.String())
		}
		require.NotContains(t, out.Body.String(), "secret")
	}
}

func TestWebtoolUsageIdentitySurvivesToolRoundContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tingly/codex/v1/responses", nil)
	c.Set(constant.CtxKeyUserID, "caller")
	c.Set(constant.CtxKeyTeamID, "team")
	SetTrackingContext(c, &typ.Rule{UUID: "rule"}, nil, "model", "requested", false)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	require.Equal(t, coretool.UsageIdentity{UserID: "caller", TeamID: "team", Scenario: "codex", RuleUUID: "rule"}, coretool.GetUsageIdentity(ctx))
}
