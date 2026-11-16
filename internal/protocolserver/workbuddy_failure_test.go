package protocolserver

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestWorkBuddyFailureRouting(t *testing.T) {
	for _, tc := range []struct {
		name, kind, message string
		status              int
		wantCalls           []string
		authError           bool
	}{
		{"channel", "workbuddy_channel_refused", "unapproved channel", 400, []string{"wb", "other"}, false},
		{"restricted", "workbuddy_account_restricted", "account restricted", 403, []string{"wb", "other"}, true},
		{"shape", "workbuddy_error", "first message is not system prompt", 400, []string{"wb"}, false},
		{"unrelated numeric code", "", "11128", 400, []string{"wb"}, false},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/complete", true: "/stream"}[streaming], func(t *testing.T) {
				loadbalance.DefaultBreakerStore().Reset()
				defer loadbalance.DefaultBreakerStore().Reset()
				cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
				require.NoError(t, err)
				wb := &typ.Provider{UUID: "wb", Name: "WorkBuddy", Enabled: true, APIStyle: "openai", APIBase: "https://example.invalid/v2", AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddyCN}}
				for _, p := range []*typ.Provider{wb, {UUID: "other", Name: "Other", Enabled: true, APIStyle: "openai", APIBase: "https://example.invalid/v1"}} {
					require.NoError(t, cfg.AddProvider(p))
				}
				rule := &typ.Rule{UUID: "workbuddy-test", LBTactic: typ.Tactic{Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()}, Services: []*loadbalance.Service{
					{Provider: "wb", Model: "primary", Active: true, Tier: 0}, {Provider: "other", Model: "m", Active: true, Tier: 1},
				}}
				loadbalance.DefaultBreakerStore().Get(rule.UUID, "wb/primary").FailureThreshold = 1
				hm := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
				h := NewHandler(ProtocolHandlerDeps{Config: cfg, LoadBalancer: NewLoadBalancer(cfg, routing.NewHealthFilter(hm)), HealthMonitor: hm})
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				var calls []string
				h.DispatchWithPriorityFailover(c, rule, wb, "primary", func(p *typ.Provider, model string) {
					calls = append(calls, p.UUID)
					if p.UUID == "wb" {
						err := &openai.Error{StatusCode: tc.status, Type: tc.kind, Code: "11128", Message: tc.message}
						if streaming {
							stream.SendStreamingError(c, err)
						} else {
							SendErrorResponse(c, err, "Failed to forward request")
						}
						return
					}
					c.String(200, "ok")
					CommitFirstChunkIfGate(c.Writer)
				})
				require.Equal(t, tc.wantCalls, calls)
				if len(calls) > 1 {
					require.Equal(t, "ok", rec.Body.String())
				} else {
					require.Equal(t, tc.status, rec.Code)
				}
				require.Equal(t, tc.authError, hm.GetHealth("wb/primary").AuthError)
				if !tc.authError {
					require.Equal(t, loadbalance.BreakerClosed, loadbalance.DefaultBreakerStore().Get(rule.UUID, "wb/primary").State())
					require.True(t, hm.IsHealthy("wb/primary"))
				}
			})
		}
	}
}
