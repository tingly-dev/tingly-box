package protocoltest

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

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
		{name: "stage keeps v1 distinct", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, wantHeader: "legacy"},
		{name: "stage beta to chat stays legacy", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, wantHeader: "legacy"},
		{name: "stage unsupported chat identity stays legacy", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, wantHeader: "legacy"},
		{
			name:       "stage beta keeps MCP on legacy",
			opts:       []TestEnvOption{NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithMCP()},
			source:     protocol.TypeAnthropicBeta,
			target:     protocol.TypeAnthropicBeta,
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
