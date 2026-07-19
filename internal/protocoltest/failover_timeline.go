package protocoltest

// Timeline failover fixtures: switchable vmodel upstreams whose availability
// can be flipped at runtime, so a test can script a wall-clock scenario like
// "00:00 all up → 00:05 vm1 down (429/500/529) → vm1 up again → traffic must
// return to vm1 after breaker recovery". Complements failover.go, whose error
// mocks are statically always-failing.

import (
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// SwitchableUpstream is a vmodel-backed httptest provider that can be flipped
// between healthy (serves the registered virtual models) and down (every
// request — including probes — gets the configured HTTP status with an error
// envelope) at any point during a test.
type SwitchableUpstream struct {
	Server *httptest.Server

	hits       atomic.Int64
	downStatus atomic.Int64 // 0 = up; otherwise the HTTP status to fail with
}

// SetDown makes every subsequent request fail with the given HTTP status.
func (u *SwitchableUpstream) SetDown(status int) { u.downStatus.Store(int64(status)) }

// SetUp restores normal vmodel serving.
func (u *SwitchableUpstream) SetUp() { u.downStatus.Store(0) }

// Hits reports how many HTTP requests reached this upstream (successful or
// failed, chat or probe endpoints alike).
func (u *SwitchableUpstream) Hits() int64 { return u.hits.Load() }

// newSwitchableUpstream builds the upstream: a virtualserver with both
// protocol registries (so it serves either API style), wrapped by a counting
// middleware that injects the outage when SetDown is active.
func newSwitchableUpstream(t *testing.T) *SwitchableUpstream {
	t.Helper()
	gin.SetMode(gin.TestMode)

	u := &SwitchableUpstream{}

	svc := virtualserver.NewService()
	openaivm.RegisterDefaults(svc.GetOpenAIRegistry())
	anthropicvm.RegisterDefaults(svc.GetAnthropicRegistry())

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		u.hits.Add(1)
		if status := int(u.downStatus.Load()); status != 0 {
			// Shape follows the Anthropic error envelope; both SDKs surface
			// the HTTP status either way, which is all failover keys off.
			c.AbortWithStatusJSON(status, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "api_error",
					"message": fmt.Sprintf("simulated outage: upstream returned %d", status),
				},
			})
			return
		}
		c.Next()
	})
	svc.SetupRoutes(engine.Group("/v1"))

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	u.Server = srv
	return u
}

// TimelineFailoverRoute is the handle returned by SetupTimelineFailoverRoute.
type TimelineFailoverRoute struct {
	ModelName  string                // gateway-facing request model
	RuleUUID   string                // for breaker-store introspection
	VMs        []*SwitchableUpstream // index == tier (VMs[0] is T0)
	ServiceIDs []string              // loadbalance service IDs, index == tier
}

// timelineSuccessModel is the vmodel every tier serves while up.
const timelineSuccessModel = "echo-model"

// SetupTimelineFailoverRoute wires an N-tier rule (T0..T(n-1)) whose tiers are
// independent switchable upstreams, all serving echo-model while up. All
// providers use the source protocol's API style (homogeneous failover — the
// cross-style path has its own suite). label must be unique per test case; it
// namespaces the provider/rule UUIDs so parallel or sequential cases cannot
// collide in the shared breaker store.
func (env *TestEnv) SetupTimelineFailoverRoute(t *testing.T, source protocol.APIType, tiers int, label string) TimelineFailoverRoute {
	t.Helper()

	apiStyle := targetToAPIStyle(source)
	requestModel := fmt.Sprintf("fo-timeline-%s-%s", source, label)

	route := TimelineFailoverRoute{
		ModelName: requestModel,
		RuleUUID:  requestModel,
	}

	services := make([]*loadbalance.Service, 0, tiers)
	for i := 0; i < tiers; i++ {
		vm := newSwitchableUpstream(t)
		route.VMs = append(route.VMs, vm)

		apiBase := vm.Server.URL
		if apiStyle == protocol.APIStyleOpenAI {
			apiBase = vm.Server.URL + "/v1"
		}

		providerUUID := fmt.Sprintf("timeline-%s-vm%d", label, i+1)
		if err := env.appConfig.AddProvider(&typ.Provider{
			UUID: providerUUID, Name: providerUUID, APIBase: apiBase, APIStyle: apiStyle,
			Token: "timeline-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
		}); err != nil {
			t.Fatalf("add timeline provider vm%d: %v", i+1, err)
		}

		services = append(services, tieredService(providerUUID, timelineSuccessModel, i))
		route.ServiceIDs = append(route.ServiceIDs, loadbalance.FormatServiceID(providerUUID, timelineSuccessModel))
	}

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, timelineSuccessModel, services...)
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add timeline rule: %v", err)
	}

	return route
}
