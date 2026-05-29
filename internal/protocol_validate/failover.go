package protocol_validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// FailoverRoute is the handle returned by SetupFailoverRoute. It carries the
// gateway-facing model name (use with SendWithModel) and the primary tier's
// per-request call counter so tests can assert how many attempts hit the
// failing upstream before falloff to the success tier.
type FailoverRoute struct {
	ModelName        string
	PrimaryCallCount *atomic.Int64
	primaryServer    *httptest.Server
}

// Close shuts down the primary fail server.
func (r *FailoverRoute) Close() {
	if r.primaryServer != nil {
		r.primaryServer.Close()
	}
}

// SetupFailoverRoute registers a two-tier priority failover rule:
//   - Primary (priority=10) is a fresh httptest.Server that always responds
//     with failStatus and a minimal JSON error body. No SSE, no body content
//     content beyond the JSON error.
//   - Fallback (priority=5) is env.virtual with successScenario registered.
//
// The gateway dispatches under TacticPriority, so the primary is tried first.
// A retryable failStatus (429/5xx) triggers gate.Discard() + fallback retry.
//
// Returns a FailoverRoute with ModelName (pass to SendWithModel) and a counter
// pointing at the primary handler's atomic call count.
func (env *TestEnv) SetupFailoverRoute(
	t *testing.T,
	source, target protocol.APIType,
	successScenario Scenario,
	failStatus int,
) FailoverRoute {
	t.Helper()

	env.virtual.RegisterScenario(successScenario.toVirtualServerScenario())

	modelSuffix := successScenario.Name
	if target == protocol.TypeOpenAIResponses {
		modelSuffix = successScenario.Name + "-codex"
	} else if target == protocol.TypeOpenAIChat {
		modelSuffix = successScenario.Name + "-chat"
	}
	providerModel := fmt.Sprintf("virtual-model-%s", modelSuffix)
	requestModel := fmt.Sprintf("fo-%s-to-%s-%s-%d", source, target, successScenario.Name, failStatus)

	var counter atomic.Int64
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(failStatus)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"simulated upstream %d","type":"upstream_error","code":"failover_test"}}`, failStatus)
	}))
	t.Cleanup(primaryServer.Close)

	apiStyle := targetToAPIStyle(target)

	primaryUUID := fmt.Sprintf("virtual-primary-%s-%d", successScenario.Name, failStatus)
	primaryAPIBase := primaryServer.URL
	if apiStyle == protocol.APIStyleOpenAI {
		primaryAPIBase = primaryServer.URL + "/v1"
	}
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID:     primaryUUID,
		Name:     primaryUUID,
		APIBase:  primaryAPIBase,
		APIStyle: apiStyle,
		Token:    "primary-token",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
	})

	fallbackUUID := fmt.Sprintf("virtual-fallback-%s-%d", successScenario.Name, failStatus)
	fallbackAPIBase := env.virtual.URL()
	if apiStyle == protocol.APIStyleOpenAI {
		fallbackAPIBase = env.virtual.URL() + "/v1"
	}
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID:     fallbackUUID,
		Name:     fallbackUUID,
		APIBase:  fallbackAPIBase,
		APIStyle: apiStyle,
		Token:    "fallback-token",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
	})

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      sourceToRuleScenario(source),
		RequestModel:  requestModel,
		ResponseModel: providerModel,
		Services: []*loadbalance.Service{
			{Provider: primaryUUID, Model: providerModel, Weight: 1, Active: true, Priority: 10, TimeWindow: 300},
			{Provider: fallbackUUID, Model: providerModel, Weight: 1, Active: true, Priority: 5, TimeWindow: 300},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticPriority,
			Params: &typ.PriorityParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Active: true,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	return FailoverRoute{
		ModelName:        requestModel,
		PrimaryCallCount: &counter,
		primaryServer:    primaryServer,
	}
}

// CustomFailoverRoute lets a test wire arbitrary http.Handlers as both tiers of
// a priority rule. Used for mid-stream-failure and all-tiers-fail tests where
// the canned SetupFailoverRoute behavior doesn't fit.
type CustomFailoverRoute struct {
	ModelName        string
	PrimaryCallCount *atomic.Int64
	FallbackCallCount *atomic.Int64
}

// SetupCustomFailoverRoute registers a two-tier priority rule with caller-
// supplied http.Handlers. Both handlers are wrapped to increment a counter
// each call. Both servers receive cleanup via t.Cleanup.
func (env *TestEnv) SetupCustomFailoverRoute(
	t *testing.T,
	source, target protocol.APIType,
	primaryHandler, fallbackHandler http.Handler,
	scenarioName string,
) CustomFailoverRoute {
	t.Helper()

	var primaryCount, fallbackCount atomic.Int64

	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCount.Add(1)
		primaryHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(primaryServer.Close)

	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCount.Add(1)
		fallbackHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(fallbackServer.Close)

	apiStyle := targetToAPIStyle(target)
	providerModel := fmt.Sprintf("virtual-model-%s", scenarioName)
	requestModel := fmt.Sprintf("fo-custom-%s-to-%s-%s", source, target, scenarioName)

	primaryAPIBase := primaryServer.URL
	fallbackAPIBase := fallbackServer.URL
	if apiStyle == protocol.APIStyleOpenAI {
		primaryAPIBase = primaryServer.URL + "/v1"
		fallbackAPIBase = fallbackServer.URL + "/v1"
	}

	primaryUUID := "virtual-custom-primary-" + scenarioName
	fallbackUUID := "virtual-custom-fallback-" + scenarioName

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
		ResponseModel: providerModel,
		Services: []*loadbalance.Service{
			{Provider: primaryUUID, Model: providerModel, Weight: 1, Active: true, Priority: 10, TimeWindow: 300},
			{Provider: fallbackUUID, Model: providerModel, Weight: 1, Active: true, Priority: 5, TimeWindow: 300},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticPriority,
			Params: &typ.PriorityParams{WithinTierTactic: loadbalance.TacticRandom},
		},
		Active: true,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	return CustomFailoverRoute{
		ModelName:         requestModel,
		PrimaryCallCount:  &primaryCount,
		FallbackCallCount: &fallbackCount,
	}
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
