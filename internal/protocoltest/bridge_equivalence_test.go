package protocoltest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// On the way up, the bridges convert an Anthropic Beta request for an OpenAI
// provider exactly as BaseTransform does in the transform chain the other
// routes still use. These tests run both on the same request and require the
// same output. The way down is pinned by the wire golden snapshots of the
// Anthropic client -> OpenAI provider pairs.

func equivalenceBridges() []stage.Bridge {
	return []stage.Bridge{
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{}),
	}
}

// equivalenceRequests is a corpus of Anthropic Beta requests covering the
// fields the request converters treat differently.
var equivalenceRequests = map[string]string{
	"text": `{"model":"provider-model","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`,
	"system_tools_history": `{"model":"provider-model","max_tokens":64,
		"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}],
		"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],
		"tool_choice":{"type":"auto"},
		"messages":[{"role":"user","content":"weather?"},
			{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"location":"Paris"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"18C"},{"type":"text","text":"and tomorrow?"}]}]}`,
	"tool_choice_named": `{"model":"provider-model","max_tokens":64,
		"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"lookup"},
		"messages":[{"role":"user","content":"look it up"}]}`,
	"thinking": `{"model":"provider-model","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[{"role":"user","content":"think"},
			{"role":"assistant","content":[{"type":"thinking","thinking":"hmm","signature":"sig"},{"type":"text","text":"done"}]},
			{"role":"user","content":"again"}]}`,
	"sampling_stop": `{"model":"provider-model","max_tokens":128,"temperature":0.3,"top_p":0.9,"top_k":40,
		"stop_sequences":["END"],"metadata":{"user_id":"u-1"},
		"messages":[{"role":"user","content":"count"}]}`,
	"image": `{"model":"provider-model","max_tokens":64,
		"messages":[{"role":"user","content":[
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},
			{"type":"image","source":{"type":"url","url":"https://example.com/cat.png"}},
			{"type":"text","text":"what is this?"}]}]}`,
}

// TestBridgeRequestEquivalence pins each bridge's provider-bound request, and
// the OpenAI Chat config it carries for the vendor transforms, to what
// BaseTransform produces for the same request.
func TestBridgeRequestEquivalence(t *testing.T) {
	t.Parallel()
	for _, bridge := range equivalenceBridges() {
		for name, body := range equivalenceRequests {
			for _, streaming := range []bool{false, true} {
				bridge, name, body, streaming := bridge, name, body, streaming
				t.Run(fmt.Sprintf("%s/%s/%s", bridge.Target(), name, streamMode(streaming)), func(t *testing.T) {
					t.Parallel()
					original := decodeBeta(t, body)
					ctx := transform.NewTransformContext(original, transform.WithStreaming(streaming))
					if err := transform.NewBaseTransform(bridge.Target()).Apply(ctx); err != nil {
						t.Fatalf("BaseTransform: %v", err)
					}

					operation := stage.OperationComplete
					if streaming {
						operation = stage.OperationStream
					}
					session, err := bridge.Open(context.Background(), stage.Call{Request: decodeBeta(t, body)}, operation)
					if err != nil {
						t.Fatalf("bridge open: %v", err)
					}
					target := session.TargetCall()

					requireSameJSON(t, "provider request", mustMarshal(ctx.Request), mustMarshal(target.Request))
					if bridge.Target() == protocol.TypeOpenAIChat {
						requireSameJSON(t, "OpenAI Chat config", mustMarshal(ctx.Config.OpenAIConfig), mustMarshal(target.State.OpenAIChat))
					}
				})
			}
		}
	}
}

func decodeBeta(t *testing.T, body string) *anthropic.BetaMessageNewParams {
	t.Helper()
	var req anthropic.BetaMessageNewParams
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return &req
}

// requireSameJSON compares two JSON documents after the golden normalization
// (sorted keys, numbered IDs, blanked timestamps).
func requireSameJSON(t *testing.T, what string, baseTransform, bridged []byte) {
	t.Helper()
	want := normalizeGolden(goldenJSON(baseTransform))
	got := normalizeGolden(goldenJSON(bridged))
	if want != got {
		t.Fatalf("%s differs from BaseTransform\n--- BaseTransform\n%s\n+++ bridge\n%s", what, want, got)
	}
}
