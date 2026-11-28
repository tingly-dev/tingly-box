package protocolserver

import (
	"context"
	"errors"
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
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
	"golang.org/x/time/rate"
)

func newWebtoolHandler(t *testing.T, service *deepseek.Service) (*ProtocolHandler, *typ.MCPRuntimeConfig) {
	t.Helper()
	cfg := &typ.MCPRuntimeConfig{RequestTimeout: 30, Sources: []typ.MCPSourceConfig{{ID: "server-webtools", Command: "unused", Enabled: typ.BoolPtr(true), Tools: []string{"*"}, Env: map[string]string{
		"WEB_SEARCH_BACKEND": "deepseek", "WEB_FETCH_BACKEND": "deepseek", "WEB_SEARCH_PROVIDER_UUID": "chosen", "WEB_FETCH_PROVIDER_UUID": "chosen",
	}}}}
	runtime := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	t.Cleanup(func() { runtime.Close() })
	runtime.SetDeepSeekWebtools(service)
	return NewHandler(ProtocolHandlerDeps{MCPRuntime: runtime}), cfg
}

func webtoolOwnerAuth(c *gin.Context) {
	if c.GetHeader("Authorization") != "Bearer owner" {
		c.AbortWithStatus(401)
		return
	}
	c.Set(constant.CtxKeyAuthKind, constant.AuthKindGlobalModelToken)
	c.Set(constant.CtxKeyUserID, "owner")
	c.Next()
}

func TestDeepSeekWebGatewayPassthroughAndAuth(t *testing.T) {
	for _, op := range []string{"search", "fetch"} {
		t.Run(op, func(t *testing.T) {
			calls := 0
			payload := `{"query":"test","count":3,"extra":"preserve"}`
			response := `{"results":[{"title":"Page","summary":"summary"}],"extra":"preserve"}`
			if op == "fetch" {
				payload = `{"url":"https://example.com","extra":"preserve"}`
				response = `{"content":"markdown","extra":"preserve"}`
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer selected-key" {
					t.Error("incorrect upstream credential")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != payload {
					t.Error("payload not preserved")
				}
				fmt.Fprint(w, response)
			}))
			defer upstream.Close()
			service := &deepseek.Service{Resolve: func() *typ.Provider { t.Error("default credential must not be used"); return nil }, ResolveProvider: func(uuid string) *typ.Provider {
				if uuid != "chosen" {
					return nil
				}
				return &typ.Provider{APIBase: upstream.URL, Token: "selected-key"}
			}}
			ph, _ := newWebtoolHandler(t, service)
			router := gin.New()
			ph.RegisterRoutes(router, webtoolOwnerAuth)
			for _, authorized := range []bool{false, true} {
				req := httptest.NewRequest("POST", "/v1/tools/"+op, strings.NewReader(payload))
				if authorized {
					req.Header.Set("Authorization", "Bearer owner")
				}
				out := httptest.NewRecorder()
				router.ServeHTTP(out, req)
				if !authorized {
					require.Equal(t, 401, out.Code)
					require.Zero(t, calls)
				} else {
					require.Equal(t, 200, out.Code)
					require.JSONEq(t, response, out.Body.String())
				}
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestDeepSeekWebGatewayGatesAndSharedLimit(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"results":[{"title":"page"}],"content":"page"}`)
	}))
	defer upstream.Close()
	service := &deepseek.Service{ResolveProvider: func(uuid string) *typ.Provider {
		if uuid != "chosen" {
			return nil
		}
		return &typ.Provider{APIBase: upstream.URL, Token: "key"}
	}}
	ph, cfg := newWebtoolHandler(t, service)
	router := gin.New()
	ph.RegisterRoutes(router, webtoolOwnerAuth)
	// All registered aliases use the same handler/limiter, not a new limiter per route.
	ph.SetupMixinEndpoints(router.Group("/test-scenario"), webtoolOwnerAuth)
	ph.SetupMixinEndpoints(router.Group("/test-scenario/v1"), webtoolOwnerAuth)
	call := func(path string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{"query":"q","url":"https://example.com"}`))
		req.Header.Set("Authorization", "Bearer owner")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out.Code
	}
	cfg.Sources[0].Enabled = typ.BoolPtr(false)
	require.Equal(t, 403, call("/v1/tools/search"))
	cfg.Sources[0].Enabled = typ.BoolPtr(true)
	cfg.Sources[0].Tools = []string{"mcp_web_fetch"}
	require.Equal(t, 403, call("/v1/tools/search"))
	cfg.Sources[0].Tools = nil // empty allow list means all tools
	cfg.Sources[0].Env["WEB_SEARCH_BACKEND"] = "serper"
	require.Equal(t, 403, call("/v1/tools/search"))
	cfg.Sources[0].Env["WEB_SEARCH_BACKEND"] = "deepseek"
	cfg.Sources[0].Env["WEB_SEARCH_PROVIDER_UUID"] = "missing"
	require.Equal(t, 403, call("/v1/tools/search"))
	cfg.Sources[0].Env["WEB_SEARCH_PROVIDER_UUID"] = "chosen"
	require.Zero(t, calls)
	ph.webtoolsLimiter = rate.NewLimiter(0, 2)
	require.Equal(t, 200, call("/v1/tools/search"))
	require.Equal(t, 200, call("/test-scenario/tools/fetch"))
	require.Equal(t, 429, call("/test-scenario/v1/tools/search"))
	require.Equal(t, 429, call("/v1/tools/fetch"))
	require.Equal(t, 2, calls)
}

type failingWebBody struct{}

func (failingWebBody) Read([]byte) (int, error) { return 0, errors.New("read failure") }
func (failingWebBody) Close() error             { return nil }

func TestDeepSeekWebGatewayErrorsAndDelegation(t *testing.T) {
	response := `{"error":{"message":"insufficient balance","code":"insufficient_quota"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(402); fmt.Fprint(w, response) }))
	defer upstream.Close()
	service := &deepseek.Service{ResolveProvider: func(string) *typ.Provider { return &typ.Provider{APIBase: upstream.URL, Token: "secret"} }}
	for _, tc := range []struct {
		name, team, kind string
		delegated        bool
		body             io.Reader
		want             int
	}{
		{"balance", "", constant.AuthKindGlobalModelToken, false, strings.NewReader(`{}`), 402},
		{"team", "team", constant.AuthKindGlobalModelToken, false, strings.NewReader(`{}`), 403},
		{"other principal", "", "user", false, strings.NewReader(`{}`), 403},
		{"delegated", "", constant.AuthKindGlobalModelToken, true, strings.NewReader(`{}`), 403},
		{"oversize", "", constant.AuthKindGlobalModelToken, false, strings.NewReader(strings.Repeat("x", 1024*1024+1)), 413},
		{"read failure", "", constant.AuthKindGlobalModelToken, false, failingWebBody{}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ph, _ := newWebtoolHandler(t, service)
			r := gin.New()
			r.POST("/tools", func(c *gin.Context) {
				c.Set(constant.CtxKeyTeamID, tc.team)
				c.Set(constant.CtxKeyAuthKind, tc.kind)
				c.Set(constant.CtxKeyEnterpriseContextVerified, tc.delegated)
			}, ph.deepSeekWebHandler("search"))
			out := httptest.NewRecorder()
			r.ServeHTTP(out, httptest.NewRequest("POST", "/tools", tc.body))
			require.Equal(t, tc.want, out.Code)
			if tc.want == 402 {
				require.JSONEq(t, response, out.Body.String())
			}
			require.NotContains(t, out.Body.String(), "secret")
		})
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
