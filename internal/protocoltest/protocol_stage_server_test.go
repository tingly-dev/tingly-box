package protocoltest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestServerProtocolStageSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		opts              []TestEnvOption
		source            protocol.APIType
		target            protocol.APIType
		streaming         bool
		wantHeader        string
		wantResponseModel bool
	}{
		{name: "default chat route legacy", source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta, wantHeader: "legacy"},
		{name: "stage chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta, wantHeader: "stage"},
		{name: "stage chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta, streaming: true, wantHeader: "stage"},
		{name: "stage beta native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to beta nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to beta stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage unsupported chat identity stays legacy", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, wantHeader: "legacy"},
		{name: "stage unsupported responses to chat stays legacy", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat, wantHeader: "legacy"},
		{
			name:       "stage beta keeps MCP on legacy",
			opts:       []TestEnvOption{NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithMCP()},
			source:     protocol.TypeAnthropicBeta,
			target:     protocol.TypeAnthropicBeta,
			wantHeader: "legacy",
		},
		{
			name:       "stage v1 keeps MCP on legacy",
			opts:       []TestEnvOption{NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithMCP()},
			source:     protocol.TypeAnthropicV1,
			target:     protocol.TypeAnthropicV1,
			wantHeader: "legacy",
		},
		{
			name:       "stage responses keeps MCP on legacy",
			opts:       []TestEnvOption{NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithMCP()},
			source:     protocol.TypeOpenAIResponses,
			target:     protocol.TypeOpenAIResponses,
			wantHeader: "legacy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t, tt.opts...)
			scenario := TextScenario()
			env.SetupRoute(tt.source, tt.target, scenario)
			model := env.findRouteModel(tt.source, tt.target, scenario.Name)
			path, body := buildRequest(tt.source, model, tt.streaming)
			req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+path, bytes.NewReader(body))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+env.ModelToken())
			req.Header.Set("X-Tingly-Debug-Routing", "1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			if got := resp.Header.Get("X-Tingly-Protocol-Pipeline"); got != tt.wantHeader {
				t.Fatalf("pipeline header = %q, want %q", got, tt.wantHeader)
			}
			if got := resp.Header.Get("X-Tingly-Upstream-API"); got != string(tt.target) {
				t.Fatalf("upstream API = %q", got)
			}
			if tt.wantResponseModel && !strings.Contains(string(responseBody), `"model":"`+model+`"`) {
				t.Fatalf("response does not expose request model %q: %s", model, responseBody)
			}
		})
	}
}

func TestServerProtocolStageAnthropicBetaGuardrailComplete(t *testing.T) {
	t.Parallel()

	for _, target := range []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			t.Parallel()
			runtime := newProtocolStageGuardrails(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
				if input.Direction == guardrailscore.DirectionResponse {
					return protocolStageBlockedResult("response denied"), nil
				}
				return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
			})
			env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithGuardrails(runtime))
			scenario := TextScenario()
			env.SetupRoute(protocol.TypeAnthropicBeta, target, scenario)
			model := env.findRouteModel(protocol.TypeAnthropicBeta, target, scenario.Name)
			path, body := buildRequest(protocol.TypeAnthropicBeta, model, false)

			resp, responseBody := sendProtocolStageProbe(t, env, path, body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, responseBody)
			}
			if got := resp.Header.Get("X-Tingly-Protocol-Pipeline"); got != "stage" {
				t.Fatalf("pipeline header = %q, want stage", got)
			}
			if !strings.Contains(string(responseBody), "Blocked by guardrails") {
				t.Fatalf("response was not blocked: %s", responseBody)
			}
		})
	}
}

func TestServerProtocolStageAnthropicBetaGuardrailStream(t *testing.T) {
	t.Parallel()

	runtime := newProtocolStageGuardrails(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
		if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil {
			return protocolStageBlockedResult("command denied"), nil
		}
		return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
	})
	env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithGuardrails(runtime))
	scenario := StreamingToolUseScenario()
	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, scenario)
	model := env.findRouteModel(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, scenario.Name)
	path, body := buildRequest(protocol.TypeAnthropicBeta, model, true)

	resp, responseBody := sendProtocolStageProbe(t, env, path, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, responseBody)
	}
	if got := resp.Header.Get("X-Tingly-Protocol-Pipeline"); got != "stage" {
		t.Fatalf("pipeline header = %q, want stage", got)
	}
	if !strings.Contains(string(responseBody), "Blocked by guardrails") {
		t.Fatalf("stream was not blocked: %s", responseBody)
	}
	if strings.Contains(string(responseBody), `"type":"tool_use"`) {
		t.Fatalf("blocked tool_use leaked to client: %s", responseBody)
	}
}

type protocolStageGuardrailPolicy func(context.Context, guardrailscore.Input) (guardrailscore.Result, error)

func (policy protocolStageGuardrailPolicy) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	return policy(ctx, input)
}

func newProtocolStageGuardrails(policy protocolStageGuardrailPolicy) *guardrails.Guardrails {
	return &guardrails.Guardrails{Policy: policy, HasActivePolicies: true}
}

func protocolStageBlockedResult(reason string) guardrailscore.Result {
	return guardrailscore.Result{
		Verdict: guardrailscore.VerdictBlock,
		Reasons: []guardrailscore.PolicyResult{{
			PolicyID: "protocol-stage-test",
			Verdict:  guardrailscore.VerdictBlock,
			Reason:   reason,
		}},
	}
}

func sendProtocolStageProbe(t *testing.T, env *TestEnv, path string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.ModelToken())
	req.Header.Set("X-Tingly-Debug-Routing", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		t.Fatalf("read response: %v", err)
	}
	return resp, responseBody
}

func TestServerProtocolStagePreservesSkipUsageFlag(t *testing.T) {
	t.Parallel()

	for _, streaming := range []bool{false, true} {
		name := "nonstream"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage())
			scenario := TextScenario()
			model := env.SetupRouteWithFlags(
				protocol.TypeOpenAIChat,
				protocol.TypeAnthropicBeta,
				scenario,
				typ.RuleFlags{SkipUsage: true},
			)
			path, body := buildRequest(protocol.TypeOpenAIChat, model, streaming)
			result, err := env.dispatch(
				protocol.TypeOpenAIChat,
				protocol.TypeAnthropicBeta,
				scenario.Name,
				path,
				body,
				map[string]string{"X-Tingly-Debug-Routing": "1"},
				streaming,
			)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if result.HTTPStatus != http.StatusOK {
				t.Fatalf("status = %d", result.HTTPStatus)
			}
			if strings.Contains(string(result.RawBody), `"usage"`) {
				t.Fatalf("response contains usage: %s", result.RawBody)
			}
		})
	}
}

func TestServerProtocolStageAnthropicBetaPreservesRuleTransforms(t *testing.T) {
	t.Parallel()

	t.Run("thinking effort", func(t *testing.T) {
		t.Parallel()
		env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage())
		scenario := flagScenario()
		model := env.SetupRouteWithFlags(
			protocol.TypeAnthropicBeta,
			protocol.TypeAnthropicBeta,
			scenario,
			typ.RuleFlags{ThinkingEffort: typ.ThinkingEffortHigh},
		)
		sendFlag(t, env, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, model, false, nil, nil)
		thinking, ok := env.virtual.LastRequest(EndpointAnthropic).JSON()["thinking"].(map[string]any)
		if !ok || thinking["type"] != "enabled" {
			t.Fatalf("upstream thinking = %#v, want enabled", thinking)
		}
	})

	t.Run("clean header", func(t *testing.T) {
		t.Parallel()
		env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage())
		scenario := flagScenario()
		model := env.SetupRouteWithFlags(
			protocol.TypeAnthropicBeta,
			protocol.TypeAnthropicBeta,
			scenario,
			typ.RuleFlags{CleanHeader: true},
		)
		sendFlag(t, env, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, model, false, func(request map[string]any) {
			request["system"] = []map[string]any{
				{"type": "text", "text": "x-anthropic-billing-header: secret-token"},
				{"type": "text", "text": "You are a helpful assistant."},
			}
		}, nil)
		upstream := string(env.virtual.LastRequest(EndpointAnthropic).Body)
		if strings.Contains(upstream, "x-anthropic-billing-header") {
			t.Fatalf("billing header survived Stage transform: %s", truncate(upstream, 300))
		}
		if !strings.Contains(upstream, "You are a helpful assistant.") {
			t.Fatalf("normal system content was removed: %s", truncate(upstream, 300))
		}
	})
}
