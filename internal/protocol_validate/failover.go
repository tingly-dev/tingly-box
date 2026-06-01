package protocol_validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/typ"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Built-in failing mock IDs registered by vmodel.RegisterErrorMocks. Tests
// pass one of these as the primary tier to drive a deterministic failure
// without writing handler scaffolding.
const (
	FailMockPreContent429 = "virtual-fail-precontent-429"
	FailMockPreContent500 = "virtual-fail-precontent-500"
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
// primary always trips the named injection (RegisterErrorMocks IDs above), the
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

	env.virtual.RegisterScenario(successScenario.toVirtualServerScenario())

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

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      sourceToRuleScenario(source),
		RequestModel:  requestModel,
		ResponseModel: fallbackProviderModel,
		Services: []*loadbalance.Service{
			{Provider: primaryUUID, Model: primaryFailModel, Weight: 1, Active: true, Tier: 0, TimeWindow: 300},
			{Provider: fallbackUUID, Model: fallbackProviderModel, Weight: 1, Active: true, Tier: 1, TimeWindow: 300},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Active: true,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	return FailoverRoute{
		ModelName:        requestModel,
		PrimaryCallCount: primaryCount,
	}
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

	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(typ.Rule{
		UUID:          requestModel,
		Scenario:      sourceToRuleScenario(source),
		RequestModel:  requestModel,
		ResponseModel: failModel,
		Services: []*loadbalance.Service{
			{Provider: primaryUUID, Model: failModel, Weight: 1, Active: true, Tier: 0, TimeWindow: 300},
			{Provider: fallbackUUID, Model: failModel, Weight: 1, Active: true, Tier: 1, TimeWindow: 300},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Active: true,
	})

	return FailoverRoute{
		ModelName:         requestModel,
		PrimaryCallCount:  primaryCount,
		FallbackCallCount: fallbackCount,
	}
}

// startFailingProvider spins up a vmodel-backed httptest.Server with both
// per-protocol registries populated by RegisterErrorMocks, wrapped with a
// per-request counter. Caller selects the desired failure shape via the
// Service.Model field on the rule (e.g. virtual-fail-precontent-429). Both
// registries are populated, so the same server serves either OpenAI Chat or
// Anthropic Messages depending on the route the gateway picks.
func startFailingProvider(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	openaivm.RegisterErrorMocks(svc.GetOpenAIRegistry())
	anthropicvm.RegisterErrorMocks(svc.GetAnthropicRegistry())

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
	result := &RoundTripResult{
		SourceProtocol: source,
		ScenarioName:   modelName,
		IsStreaming:    streaming,
	}

	if streaming {
		url := env.gatewayServer.URL + path
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.modelToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do streaming request: %v", err)
		}
		defer resp.Body.Close()

		result.HTTPStatus = resp.StatusCode
		result.StreamEvents, result.RawBody = sse.ReadSSELines(resp.Body)
		parsed := assembleFromEvents(result.StreamEvents, sourceToStyle(source))
		fillFromParsedResult(result, parsed, sourceToStyle(source), true)
	} else {
		req, err := http.NewRequest("POST", path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.modelToken)

		w := httptest.NewRecorder()
		env.ginEngine.ServeHTTP(w, req)
		result.HTTPStatus = w.Code
		result.RawBody = w.Body.Bytes()
		parsed := parseFromJSON(result.RawBody, sourceToStyle(source))
		fillFromParsedResult(result, parsed, sourceToStyle(source), false)
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
