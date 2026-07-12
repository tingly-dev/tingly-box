package protocoltest

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Built-in failing mock IDs registered by vmodel.SharedDefaultMocks. Tests
// pass one of these as the primary tier to drive a deterministic failure
// without writing handler scaffolding.
const (
	FailMockPreContent429 = "virtual-fail-429"
	FailMockPreContent500 = "virtual-fail-500"
	FailMockMidStreamCut  = "virtual-fail-midstream-close"
	FailMockMidStreamErr  = "virtual-fail-midstream-event"
)

// FailoverRoute is the handle returned by SetupFailoverRoute / SetupBothFailingRoute.
// ModelName is the gateway-facing model (pass to SendWithModel).
// PrimaryCallCount tracks how often the primary tier was hit. FallbackCallCount
// is non-nil only when both tiers run through vmodel mock servers (i.e.
// SetupBothFailingRoute); otherwise it is nil and tests must rely on content
// discrimination for fallback assertions.
type FailoverRoute struct {
	ModelName         string
	PrimaryCallCount  *atomic.Int64
	FallbackCallCount *atomic.Int64 // nil when fallback is env.virtual
}

// SetupFailoverRoute wires a two-tier rule using vmodel's pre-registered error
// mocks for the primary tier. Both tiers run inside httptest servers; the
// primary always trips the named injection (SharedDefaultMocks IDs above), the
// fallback serves successScenario via env.virtual.
//
// The orchestrator dispatches under TacticTier; the primary (Tier 0) is tried
// first; pre-content failures are retryable; mid-stream failures commit the
// gate (no retry). This is the single helper that covers all failover-test
// shapes: 429/500 pre-content, mid-stream close, mid-stream event.
func (env *TestEnv) SetupFailoverRoute(
	t *testing.T,
	source, target protocol.APIType,
	successScenario Scenario,
	primaryFailModel string,
) FailoverRoute {
	t.Helper()

	env.virtual.RegisterScenario(successScenario)

	modelSuffix := successScenario.Name
	switch target {
	case protocol.TypeOpenAIResponses:
		modelSuffix = successScenario.Name + "-codex"
	case protocol.TypeOpenAIChat:
		modelSuffix = successScenario.Name + "-chat"
	}
	fallbackProviderModel := fmt.Sprintf("virtual-model-%s", modelSuffix)
	requestModel := fmt.Sprintf("fo-%s-to-%s-%s-%s", source, target, successScenario.Name, primaryFailModel)

	apiStyle := targetToAPIStyle(target)
	primaryServer, primaryCount := startFailingProvider(t)

	primaryAPIBase := primaryServer.URL
	fallbackAPIBase := env.virtual.URL()
	if apiStyle == protocol.APIStyleOpenAI {
		primaryAPIBase = primaryServer.URL + "/v1"
		fallbackAPIBase = env.virtual.URL() + "/v1"
	}

	primaryUUID := fmt.Sprintf("virtual-primary-%s-%s", successScenario.Name, primaryFailModel)
	fallbackUUID := fmt.Sprintf("virtual-fallback-%s-%s", successScenario.Name, primaryFailModel)

	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: primaryUUID, Name: primaryUUID, APIBase: primaryAPIBase, APIStyle: apiStyle,
		Token: "primary-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: fallbackUUID, Name: fallbackUUID, APIBase: fallbackAPIBase, APIStyle: apiStyle,
		Token: "fallback-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, fallbackProviderModel,
		tieredService(primaryUUID, primaryFailModel, 0),
		tieredService(fallbackUUID, fallbackProviderModel, 1))
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add failover rule: %v", err)
	}

	return FailoverRoute{
		ModelName:        requestModel,
		PrimaryCallCount: primaryCount,
	}
}

// UpstreamEndpointHits exposes env.virtual's endpoint-hit counter to
// out-of-package (_test) callers — e.g. to assert a cross-style failover reached
// the fallback on the expected provider-native endpoint.
func (env *TestEnv) UpstreamEndpointHits(kind EndpointKind) int {
	return env.virtual.EndpointHits(kind)
}

// UpstreamLastRequest exposes the last request env.virtual captured on an
// endpoint, so tests can assert the re-transformed upstream wire shape.
func (env *TestEnv) UpstreamLastRequest(kind EndpointKind) *CapturedRequest {
	return env.virtual.LastRequest(kind)
}

// SetupCrossStyleFailoverRoute wires a two-tier rule whose tiers use DIFFERENT
// API styles: the primary (primaryStyle, a vmodel error server) trips a
// pre-content failure, and the fallback (fallbackTarget's style, served by
// env.virtual) succeeds. The gateway receives one `source` request; the
// orchestrator must re-transform it into primaryStyle's wire format for the
// first attempt and, after failover, into the fallback's wire format for the
// second — the core guarantee of the lifted failover. env.virtual captures the
// fallback's request so the test can assert the re-transformed wire shape.
func (env *TestEnv) SetupCrossStyleFailoverRoute(
	t *testing.T,
	source protocol.APIType,
	primaryStyle protocol.APIStyle,
	fallbackTarget protocol.APIType,
	successScenario Scenario,
	primaryFailModel string,
) FailoverRoute {
	t.Helper()

	env.virtual.RegisterScenario(successScenario)

	modelSuffix := successScenario.Name
	switch fallbackTarget {
	case protocol.TypeOpenAIResponses:
		modelSuffix = successScenario.Name + "-codex"
	case protocol.TypeOpenAIChat:
		modelSuffix = successScenario.Name + "-chat"
	}
	fallbackProviderModel := fmt.Sprintf("virtual-model-%s", modelSuffix)
	requestModel := fmt.Sprintf("foxs-%s-%s-to-%s-%s", source, primaryStyle, fallbackTarget, primaryFailModel)

	fallbackStyle := targetToAPIStyle(fallbackTarget)
	primaryServer, primaryCount := startFailingProvider(t)

	primaryAPIBase := primaryServer.URL
	if primaryStyle == protocol.APIStyleOpenAI {
		primaryAPIBase = primaryServer.URL + "/v1"
	}
	fallbackAPIBase := env.virtual.URL()
	if fallbackStyle == protocol.APIStyleOpenAI {
		fallbackAPIBase = env.virtual.URL() + "/v1"
	}

	primaryUUID := fmt.Sprintf("virtual-xs-primary-%s-%s", successScenario.Name, primaryFailModel)
	fallbackUUID := fmt.Sprintf("virtual-xs-fallback-%s-%s", successScenario.Name, primaryFailModel)

	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: primaryUUID, Name: primaryUUID, APIBase: primaryAPIBase, APIStyle: primaryStyle,
		Token: "primary-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: fallbackUUID, Name: fallbackUUID, APIBase: fallbackAPIBase, APIStyle: fallbackStyle,
		OpenAIEndpointMode: targetToOpenAIEndpointMode(fallbackTarget),
		Token:              "fallback-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, fallbackProviderModel,
		tieredService(primaryUUID, primaryFailModel, 0),
		tieredService(fallbackUUID, fallbackProviderModel, 1))
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add failover rule: %v", err)
	}

	return FailoverRoute{
		ModelName:        requestModel,
		PrimaryCallCount: primaryCount,
	}
}

// SetupVModelFailoverRoute wires a two-tier rule where BOTH tiers are in-process
// virtual-model providers (AuthType = vmodel, #1249), so failover traverses the
// vmodel ClientPool path rather than httptest upstreams. The primary trips the
// named failing model (e.g. FailMockPreContent500) in primaryStyle; the fallback
// serves fallbackModel (e.g. "echo-model") in fallbackStyle. Set primaryStyle ≠
// fallbackStyle to exercise cross-style failover through the vmodel clients.
//
// PrimaryCallCount is nil (in-process providers have no httptest counter);
// assert on the client result instead.
func (env *TestEnv) SetupVModelFailoverRoute(
	t *testing.T,
	source protocol.APIType,
	primaryStyle, fallbackStyle protocol.APIStyle,
	primaryFailModel, fallbackModel string,
) FailoverRoute {
	t.Helper()

	requestModel := fmt.Sprintf("vmfo-%s-%s-%s-to-%s-%s", source, primaryStyle, primaryFailModel, fallbackStyle, fallbackModel)
	primaryUUID := fmt.Sprintf("vm-primary-%s-%s", primaryStyle, primaryFailModel)
	fallbackUUID := fmt.Sprintf("vm-fallback-%s-%s", fallbackStyle, fallbackModel)

	// Virtual providers mirror the builtin vmodel seed shape: a non-empty
	// sentinel APIBase (AddProvider rejects empty), AuthType=vmodel (routes to
	// the in-process client), and a VModelDetail advertising the served model.
	if err := env.appConfig.AddProvider(&typ.Provider{
		UUID: primaryUUID, Name: primaryUUID, APIBase: "vmodel://local", APIStyle: primaryStyle,
		AuthType: typ.AuthTypeVirtual, Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
		VModelDetail: &typ.VModelDetail{Models: []string{primaryFailModel}},
	}); err != nil {
		t.Fatalf("add primary vmodel provider: %v", err)
	}
	if err := env.appConfig.AddProvider(&typ.Provider{
		UUID: fallbackUUID, Name: fallbackUUID, APIBase: "vmodel://local", APIStyle: fallbackStyle,
		AuthType: typ.AuthTypeVirtual, Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
		VModelDetail: &typ.VModelDetail{Models: []string{fallbackModel}},
	}); err != nil {
		t.Fatalf("add fallback vmodel provider: %v", err)
	}

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, fallbackModel,
		tieredService(primaryUUID, primaryFailModel, 0),
		tieredService(fallbackUUID, fallbackModel, 1))
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add failover rule: %v", err)
	}

	return FailoverRoute{ModelName: requestModel}
}

// SetupBothFailingRoute wires a two-tier rule where BOTH tiers trip the same
// pre-content injection. Used for the all-tiers-fail test: client must see a
// non-200 once the orchestrator exhausts its budget.
func (env *TestEnv) SetupBothFailingRoute(
	t *testing.T,
	source, target protocol.APIType,
	failModel string,
) FailoverRoute {
	t.Helper()

	apiStyle := targetToAPIStyle(target)
	primaryServer, primaryCount := startFailingProvider(t)
	fallbackServer, fallbackCount := startFailingProvider(t)

	primaryAPIBase := primaryServer.URL
	fallbackAPIBase := fallbackServer.URL
	if apiStyle == protocol.APIStyleOpenAI {
		primaryAPIBase = primaryServer.URL + "/v1"
		fallbackAPIBase = fallbackServer.URL + "/v1"
	}

	primaryUUID := "virtual-both-fail-primary-" + failModel
	fallbackUUID := "virtual-both-fail-fallback-" + failModel
	requestModel := "fo-both-fail-" + failModel

	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: primaryUUID, Name: primaryUUID, APIBase: primaryAPIBase, APIStyle: apiStyle,
		Token: "primary-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: fallbackUUID, Name: fallbackUUID, APIBase: fallbackAPIBase, APIStyle: apiStyle,
		Token: "fallback-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})

	rule := newHarnessRule(requestModel, sourceToRuleScenario(source), requestModel, failModel,
		tieredService(primaryUUID, failModel, 0),
		tieredService(fallbackUUID, failModel, 1))
	rule.LBTactic = tierFailoverTactic()
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add failover rule: %v", err)
	}

	return FailoverRoute{
		ModelName:         requestModel,
		PrimaryCallCount:  primaryCount,
		FallbackCallCount: fallbackCount,
	}
}

// startFailingProvider spins up a vmodel-backed httptest.Server with both
// per-protocol registries populated by RegisterDefaults (which includes error models),
// wrapped with a per-request counter. Caller selects the desired failure shape via the
// Service.Model field on the rule (e.g. virtual-fail-429). Both registries are populated,
// so the same server serves either OpenAI Chat or Anthropic Messages depending on the
// route the gateway picks.
func startFailingProvider(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	// Error models are now in SharedDefaultMocks, registered by default
	openaivm.RegisterDefaults(svc.GetOpenAIRegistry())
	anthropicvm.RegisterDefaults(svc.GetAnthropicRegistry())

	var count atomic.Int64
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		count.Add(1)
		c.Next()
	})
	svc.SetupRoutes(engine.Group("/v1"))

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv, &count
}

// SendWithModel sends a request using an explicit model name (bypassing the
// SetupRoute route map). Used for failover tests where the rule's request
// model doesn't follow SetupRoute's naming convention.
func (env *TestEnv) SendWithModel(t *testing.T, source protocol.APIType, modelName string, streaming bool) *RoundTripResult {
	t.Helper()

	path, body := buildRequest(source, modelName, streaming)
	result, err := env.dispatch(source, "", modelName, path, body, nil, streaming)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return result
}

// failoverJSONBody is a convenience helper for tests that need to peek at the
// JSON error body returned by the gateway after all tiers fail.
func failoverJSONBody(b []byte) map[string]interface{} {
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}
