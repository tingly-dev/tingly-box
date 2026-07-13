package guardrail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	guardrailsruntime "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	protocol "github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func TestAnthropicBetaGuardrailStageMasksRequestAndRestoresCompleteResponse(t *testing.T) {
	t.Parallel()

	const secret = "sk-live-secret-value"
	credential := guardrailscore.ProtectedCredential{
		ID:         "credential-1",
		Name:       "test key",
		Type:       guardrailscore.ProtectedCredentialTypeAPIKey,
		Secret:     secret,
		AliasToken: "TINGLY_CRED_API_KEY_TEST",
		Enabled:    true,
	}
	runtime := testGuardrailsRuntime(policyRunnerFunc(func(context.Context, guardrailscore.Input) (guardrailscore.Result, error) {
		return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
	}))
	runtime.SetCredentialCache(guardrailsruntime.BuildCredentialCache([]guardrailscore.ProtectedCredential{credential}, []string{"anthropic"}))

	request := &anthropic.BetaMessageNewParams{
		Model:     "client-model",
		MaxTokens: 64,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("use " + secret)),
		},
	}
	var providerText string
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, protocolstage.Call) (*protocolstage.Response, error) {
			providerText = request.Messages[0].Content[0].OfText.Text
			return &protocolstage.Response{Value: &anthropic.BetaMessage{
				Content: []anthropic.BetaContentBlockUnion{{Type: "text", Text: providerText}},
			}}, nil
		},
	}
	endpoint := composeAnthropicBetaGuardrail(t, terminal, runtime)
	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if strings.Contains(providerText, secret) || !strings.Contains(providerText, credential.AliasToken) {
		t.Fatalf("provider request text = %q", providerText)
	}
	message := response.Value.(*anthropic.BetaMessage)
	if got := message.Content[0].Text; got != "use "+secret {
		t.Fatalf("client response text = %q, want restored secret", got)
	}
}

func TestAnthropicBetaGuardrailStageBlocksCompleteResponse(t *testing.T) {
	t.Parallel()

	runtime := testGuardrailsRuntime(policyRunnerFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
		if input.Direction == guardrailscore.DirectionResponse && strings.Contains(input.Content.Text, "danger") {
			return blockedResult("dangerous output"), nil
		}
		return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
	}))
	request := &anthropic.BetaMessageNewParams{Model: "client-model", MaxTokens: 64}
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, protocolstage.Call) (*protocolstage.Response, error) {
			return &protocolstage.Response{Value: &anthropic.BetaMessage{
				Content:    []anthropic.BetaContentBlockUnion{{Type: "text", Text: "danger"}},
				StopReason: anthropic.BetaStopReasonToolUse,
			}}, nil
		},
	}
	endpoint := composeAnthropicBetaGuardrail(t, terminal, runtime)
	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	message := response.Value.(*anthropic.BetaMessage)
	if len(message.Content) != 1 || message.Content[0].Type != "text" || !strings.Contains(message.Content[0].Text, "Blocked by guardrails") {
		t.Fatalf("blocked response content = %+v", message.Content)
	}
	if message.StopReason != anthropic.BetaStopReasonEndTurn {
		t.Fatalf("stop reason = %q, want end_turn", message.StopReason)
	}
}

func TestAnthropicBetaGuardrailStageRewritesBlockedToolUseStream(t *testing.T) {
	t.Parallel()

	runtime := testGuardrailsRuntime(policyRunnerFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
		if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil {
			return blockedResult("command denied"), nil
		}
		return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
	}))
	request := &anthropic.BetaMessageNewParams{Model: "client-model", MaxTokens: 64}
	target := &fakeStream{
		events: []protocolstage.Event{
			{Value: decodeBetaEvent(t, `{"type":"message_start","message":{"id":"msg-1","type":"message","role":"assistant","model":"provider","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":0}}}`)},
			{Value: decodeBetaEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool-1","name":"shell","input":{}}}`)},
			{Value: decodeBetaEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"rm -rf /\"}"}}`)},
			{Value: decodeBetaEvent(t, `{"type":"content_block_stop","index":0}`)},
			{Value: decodeBetaEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":4}}`)},
			{Value: decodeBetaEvent(t, `{"type":"message_stop"}`)},
		},
		result: protocolstage.StreamResult{Usage: protocol.NewTokenUsage(3, 4), Model: "provider"},
	}
	terminal := &fakeEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
			return target, nil
		},
	}
	endpoint := composeAnthropicBetaGuardrail(t, terminal, runtime)
	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	var payloads []map[string]any
	for {
		event, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		payloads = append(payloads, eventMap(t, event.Value))
	}
	if len(payloads) != 6 {
		t.Fatalf("output event count = %d, want 6: %#v", len(payloads), payloads)
	}
	block, _ := payloads[1]["content_block"].(map[string]any)
	if block["type"] != "text" {
		t.Fatalf("rewritten block = %#v", block)
	}
	delta, _ := payloads[2]["delta"].(map[string]any)
	if text, _ := delta["text"].(string); !strings.Contains(text, "Blocked by guardrails") {
		t.Fatalf("rewritten delta = %#v", delta)
	}
	messageDelta, _ := payloads[4]["delta"].(map[string]any)
	if messageDelta["stop_reason"] != "end_turn" {
		t.Fatalf("message delta = %#v", messageDelta)
	}
	if got := stream.Result(); got.Model != "provider" || got.Usage == nil {
		t.Fatalf("Result() = %+v", got)
	}
}

type policyRunnerFunc func(context.Context, guardrailscore.Input) (guardrailscore.Result, error)

func (f policyRunnerFunc) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	return f(ctx, input)
}

func testGuardrailsRuntime(runner guardrailsruntime.PolicyRunner) *guardrailsruntime.Guardrails {
	return &guardrailsruntime.Guardrails{Policy: runner, HasActivePolicies: true}
}

func blockedResult(reason string) guardrailscore.Result {
	return guardrailscore.Result{
		Verdict: guardrailscore.VerdictBlock,
		Reasons: []guardrailscore.PolicyResult{{
			PolicyID: "test-policy",
			Verdict:  guardrailscore.VerdictBlock,
			Reason:   reason,
		}},
	}
}

func composeAnthropicBetaGuardrail(t *testing.T, terminal protocolstage.Endpoint, runtime *guardrailsruntime.Guardrails) protocolstage.Endpoint {
	t.Helper()
	guardrail, err := NewAnthropicBeta(AnthropicBetaConfig{
		Runtime: runtime,
		BaseInput: guardrailscore.Input{
			Scenario: "anthropic",
			Model:    "provider-model",
		},
	})
	if err != nil {
		t.Fatalf("NewAnthropicBeta() error = %v", err)
	}
	endpoint, err := protocolstage.Compose(terminal, guardrail)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	return endpoint
}

func decodeBetaEvent(t *testing.T, raw string) anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var event anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("decode Beta event: %v", err)
	}
	return event
}

func eventMap(t *testing.T, value any) map[string]any {
	t.Helper()
	var raw []byte
	switch event := value.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		raw = []byte(event.RawJSON())
	case protocolstream.AnthropicEvent:
		var err error
		raw, err = json.Marshal(event.Data)
		if err != nil {
			t.Fatalf("marshal Anthropic event: %v", err)
		}
	default:
		t.Fatalf("event type = %T", value)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	return payload
}
