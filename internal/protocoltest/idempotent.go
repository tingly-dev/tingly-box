package protocoltest

import (
	"fmt"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IdempotentCase describes a round-trip idempotency check. It compares the
// client-visible result of two request paths that should be observationally
// identical:
//
//	baseline:   Client(A) ──[A→A passthrough]──────────────→ R0
//	round-trip: Client(A) ──[A→B]──[B→A]─────────────────────→ R1
//
// The round-trip chains two real conversions: the first hop converts A→B and
// forwards to the gateway itself, which re-enters through B's inbound route and
// converts B→A before hitting the mock provider. If either conversion drops
// information, R0 and R1 diverge.
//
//	g(f(A)) == A   where f = A→B, g = B→A
type IdempotentCase struct {
	Name     string           // human-readable label
	Source   protocol.APIType // A: the client-facing protocol
	Mid      protocol.APIType // B: the intermediate protocol the chain passes through
	Baseline protocol.APIType // A': passthrough target for the baseline (same API style as A)
}

// DefaultIdempotentCases returns the canonical round-trip idempotency cases:
// every pair among the three first-class protocols (Anthropic, OpenAI Chat,
// OpenAI Responses), in both directions. Chat and Responses are treated as
// distinct protocols — the chain head sets OpenAIEndpointMode so a Responses
// intermediate genuinely re-enters /responses, not /chat/completions.
//
// Baseline is the source's same-style passthrough target:
//   - anthropic_v1   → anthropic_beta (Anthropic passthrough)
//   - openai_chat    → openai_chat
//   - openai_responses → openai_responses
//
// Google is omitted: the harness's virtual-provider plumbing does not yet
// support Google as a target, so it cannot serve as an intermediate hop.
func DefaultIdempotentCases() []IdempotentCase {
	return []IdempotentCase{
		// Anthropic ↔ OpenAI Chat
		{
			Name:     "openai_chat_via_anthropic",
			Source:   protocol.TypeOpenAIChat,
			Mid:      protocol.TypeAnthropicV1,
			Baseline: protocol.TypeOpenAIChat,
		},
		{
			Name:     "anthropic_via_openai_chat",
			Source:   protocol.TypeAnthropicV1,
			Mid:      protocol.TypeOpenAIChat,
			Baseline: protocol.TypeAnthropicBeta,
		},

		// Anthropic ↔ OpenAI Responses
		{
			Name:     "openai_responses_via_anthropic",
			Source:   protocol.TypeOpenAIResponses,
			Mid:      protocol.TypeAnthropicV1,
			Baseline: protocol.TypeOpenAIResponses,
		},
		{
			Name:     "anthropic_via_openai_responses",
			Source:   protocol.TypeAnthropicV1,
			Mid:      protocol.TypeOpenAIResponses,
			Baseline: protocol.TypeAnthropicBeta,
		},

		// OpenAI Chat ↔ OpenAI Responses
		{
			Name:     "openai_chat_via_responses",
			Source:   protocol.TypeOpenAIChat,
			Mid:      protocol.TypeOpenAIResponses,
			Baseline: protocol.TypeOpenAIChat,
		},
		{
			Name:     "openai_responses_via_chat",
			Source:   protocol.TypeOpenAIResponses,
			Mid:      protocol.TypeOpenAIChat,
			Baseline: protocol.TypeOpenAIResponses,
		},
	}
}

// gatewayEntryBase returns the APIBase a chain-hop provider must use so that
// the gateway, when forwarding to it, re-enters its OWN inbound route for the
// given target protocol. The SDK appends the endpoint path (chat/completions,
// v1/messages, …) to this base — see internal/protocol routing.
func (env *TestEnv) gatewayEntryBase(target protocol.APIType) string {
	gw := env.gatewayServer.URL
	switch targetToAPIStyle(target) {
	case protocol.APIStyleAnthropic:
		// SDK appends "v1/messages" → /tingly/anthropic/v1/messages
		return gw + "/tingly/anthropic"
	default:
		// OpenAI: SDK appends "chat/completions" → /tingly/openai/v1/chat/completions
		return gw + "/tingly/openai/v1"
	}
}

// setupChainHopRoute configures a route whose upstream provider is the gateway
// itself (not the virtual server). A request matching requestModel is converted
// source→target and forwarded back into the gateway carrying nextModel, where
// the next hop's route picks it up. The provider token is the gateway's own
// model token so the re-entry passes authentication.
func (env *TestEnv) setupChainHopRoute(source, target protocol.APIType, s Scenario, requestModel, nextModel string) {
	providerName := fmt.Sprintf("chain-%s-%s-%s", source, target, s.Name)

	provider := &typ.Provider{
		UUID:               providerName,
		Name:               providerName,
		APIBase:            env.gatewayEntryBase(target),
		APIStyle:           targetToAPIStyle(target),
		OpenAIEndpointMode: targetToOpenAIEndpointMode(target), // re-enter the right OpenAI endpoint
		Token:              env.modelToken,                     // re-entry into the gateway must authenticate
		Enabled:            true,
		Timeout:            int64(constant.DefaultRequestTimeout),
	}
	_ = env.appConfig.AddProvider(provider)

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      sourceToRuleScenario(source),
		RequestModel:  requestModel,
		ResponseModel: nextModel,
		Services: []*loadbalance.Service{
			{
				Provider:   providerName,
				Model:      nextModel,
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Active: true,
	}
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)
}

// RunIdempotent executes round-trip idempotency tests for all cases × scenarios.
// For each case it wires three routes in a single gateway, drives the baseline
// and round-trip requests, and asserts the client-visible results are
// semantically equivalent.
func (m *Matrix) RunIdempotent(t *testing.T) {
	t.Helper()

	cases := DefaultIdempotentCases()

	for _, scenario := range m.Scenarios {
		scenario := scenario
		// Error propagation through two hops is out of scope: the inner
		// gateway wraps upstream errors, so the byte-for-byte error shape is
		// not expected to survive a round-trip.
		if scenario.Name == "error" {
			continue
		}

		t.Run(scenario.Name, func(t *testing.T) {
			t.Parallel()

			env := NewTestEnv(t)
			defer env.Close()

			for _, ic := range cases {
				ic := ic
				for _, streaming := range m.Streaming {
					streaming := streaming
					modeSuffix := "nonstream"
					if streaming {
						modeSuffix = "stream"
					}
					label := fmt.Sprintf("%s/%s", ic.Name, modeSuffix)

					t.Run(label, func(t *testing.T) {
						// Both hops must be supported for this scenario.
						if reason, skip := skipIdempotentScenario(ic, scenario.Name); skip {
							t.Skipf("skipped: %s", reason)
							return
						}
						if streaming && !scenarioSupportsStreaming(scenario) {
							t.Skip("scenario does not support streaming")
							return
						}
						if !streaming && scenarioRequiresStreaming(scenario) {
							t.Skip("scenario requires streaming mode")
							return
						}

						// Baseline: A → A' passthrough through the virtual server.
						env.SetupRoute(ic.Source, ic.Baseline, scenario)
						baseModel := env.findRouteModel(ic.Source, ic.Baseline, scenario.Name)

						// Chain tail: B → A' through the virtual server.
						env.SetupRoute(ic.Mid, ic.Baseline, scenario)
						tailModel := env.findRouteModel(ic.Mid, ic.Baseline, scenario.Name)

						// Chain head: A → B, forwarding back into the gateway
						// carrying tailModel.
						headModel := fmt.Sprintf("idem-%s-%s", ic.Name, scenario.Name)
						env.setupChainHopRoute(ic.Source, ic.Mid, scenario, headModel, tailModel)

						baseline, err := env.sendModel(ic.Source, ic.Baseline, scenario.Name, baseModel, streaming)
						if err != nil {
							t.Fatalf("baseline send: %v", err)
						}
						roundtrip, err := env.sendModel(ic.Source, ic.Mid, scenario.Name, headModel, streaming)
						if err != nil {
							t.Fatalf("round-trip send: %v", err)
						}

						// Each path must independently satisfy the scenario.
						for _, a := range scenario.Assertions {
							if err := a.Check(baseline); err != nil {
								t.Errorf("baseline (%s→%s) assertion %q failed: %v\n  body: %s",
									ic.Source, ic.Baseline, a.Name, err, truncate(string(baseline.RawBody), 300))
							}
							if err := a.Check(roundtrip); err != nil {
								t.Errorf("round-trip (%s→%s→%s) assertion %q failed: %v\n  body: %s",
									ic.Source, ic.Mid, ic.Baseline, a.Name, err, truncate(string(roundtrip.RawBody), 300))
							}
						}

						// The whole point: baseline and round-trip must agree.
						chainLabel := fmt.Sprintf("%s→%s→%s vs %s→%s",
							ic.Source, ic.Mid, ic.Baseline, ic.Source, ic.Baseline)
						assertSemanticEquivalence(t, chainLabel, baseline, roundtrip)
					})
				}
			}
		})
	}
}

// skipIdempotentScenario reports whether a case+scenario should be skipped
// because one of its hops is in the single-hop skip list.
func skipIdempotentScenario(ic IdempotentCase, scenarioName string) (string, bool) {
	for _, src := range []protocol.APIType{ic.Source, ic.Mid} {
		key := fmt.Sprintf("%s|%s", src, scenarioName)
		if reason, skip := skipSourceScenarios[key]; skip {
			return reason, true
		}
	}
	return "", false
}
