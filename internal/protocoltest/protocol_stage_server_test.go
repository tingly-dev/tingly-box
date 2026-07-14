package protocoltest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
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
		{name: "stage chat to responses nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIResponses, wantHeader: "stage", wantResponseModel: true},
		{name: "stage chat to responses stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIResponses, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage chat identity nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage chat identity stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to responses nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIResponses, wantHeader: "stage", wantResponseModel: true},
		{name: "stage beta to responses stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIResponses, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to responses nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIResponses, wantHeader: "stage", wantResponseModel: true},
		{name: "stage v1 to responses stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIResponses, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses native nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses native stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to beta nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to beta stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, streaming: true, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to chat nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat, wantHeader: "stage", wantResponseModel: true},
		{name: "stage responses to chat stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat, streaming: true, wantHeader: "stage", wantResponseModel: true},
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

func TestServerProtocolStageRecordingSelection(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		source     protocol.APIType
		target     protocol.APIType
		wantHeader string
	}{
		{name: "beta identity", source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, wantHeader: "stage"},
		{name: "beta to chat", source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, wantHeader: "stage"},
		{name: "beta to responses", source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIResponses, wantHeader: "stage"},
		{name: "v1 identity", source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, wantHeader: "stage"},
		{name: "v1 to chat", source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, wantHeader: "stage"},
		{name: "v1 to responses", source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIResponses, wantHeader: "stage"},
		{name: "chat identity", source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, wantHeader: "stage"},
		{name: "chat to beta", source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta, wantHeader: "stage"},
		{name: "chat to responses", source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIResponses, wantHeader: "stage"},
		{name: "responses identity", source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, wantHeader: "stage"},
		{name: "responses to beta", source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, wantHeader: "stage"},
		{name: "responses to chat", source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat, wantHeader: "stage"},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t,
				NewTestEnvOptionWithProtocolStage(),
				NewTestEnvOptionWithRecordDir(t.TempDir()),
			)
			scenario := TextScenario()
			env.SetupRoute(tt.source, tt.target, scenario)
			model := env.findRouteModel(tt.source, tt.target, scenario.Name)
			path, body := buildRequest(tt.source, model, false)

			resp, responseBody := sendProtocolStageProbe(t, env, path, body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, responseBody)
			}
			if got := resp.Header.Get("X-Tingly-Protocol-Pipeline"); got != tt.wantHeader {
				t.Fatalf("pipeline header = %q, want %q", got, tt.wantHeader)
			}
		})
	}
}

func TestServerProtocolStageRecordingFailover(t *testing.T) {
	routes := []struct {
		name           string
		source         protocol.APIType
		primaryStyle   protocol.APIStyle
		fallbackTarget protocol.APIType
		firstProtocol  protocol.APIType
		secondProtocol protocol.APIType
	}{
		{name: "beta", source: protocol.TypeAnthropicBeta, primaryStyle: protocol.APIStyleAnthropic, fallbackTarget: protocol.TypeOpenAIChat, firstProtocol: protocol.TypeAnthropicBeta, secondProtocol: protocol.TypeOpenAIChat},
		{name: "v1", source: protocol.TypeAnthropicV1, primaryStyle: protocol.APIStyleAnthropic, fallbackTarget: protocol.TypeOpenAIChat, firstProtocol: protocol.TypeAnthropicV1, secondProtocol: protocol.TypeOpenAIChat},
		{name: "chat", source: protocol.TypeOpenAIChat, primaryStyle: protocol.APIStyleOpenAI, fallbackTarget: protocol.TypeAnthropicBeta, firstProtocol: protocol.TypeOpenAIChat, secondProtocol: protocol.TypeAnthropicBeta},
		{name: "responses", source: protocol.TypeOpenAIResponses, primaryStyle: protocol.APIStyleOpenAI, fallbackTarget: protocol.TypeAnthropicBeta, firstProtocol: protocol.TypeOpenAIChat, secondProtocol: protocol.TypeAnthropicBeta},
	}
	for _, routeCase := range routes {
		routeCase := routeCase
		for _, streaming := range []bool{false, true} {
			streaming := streaming
			name := routeCase.name + "/complete"
			if streaming {
				name = routeCase.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				recordDir := t.TempDir()
				env := NewTestEnv(t,
					NewTestEnvOptionWithProtocolStage(),
					NewTestEnvOptionWithRecordDir(recordDir),
				)
				scenario := TextScenario()
				if streaming {
					scenario = StreamingTextScenario()
				}
				route := env.SetupCrossStyleFailoverRoute(
					t,
					routeCase.source,
					routeCase.primaryStyle,
					routeCase.fallbackTarget,
					scenario,
					FailMockPreContent500,
				)

				result := env.SendWithModel(t, routeCase.source, route.ModelName, streaming)
				if result.HTTPStatus != http.StatusOK {
					t.Fatalf("status = %d", result.HTTPStatus)
				}
				env.Close()

				records := readPersistedRequestRecords(t, recordDir)
				if len(records) != 1 {
					t.Fatalf("RequestRecord count = %d, want 1", len(records))
				}
				record := records[0]
				if record.Outcome != requestrecord.OutcomeSucceeded {
					t.Fatalf("request outcome = %q, want succeeded", record.Outcome)
				}
				if len(record.ProviderExchanges) != 2 {
					t.Fatalf("provider exchange count = %d, want 2", len(record.ProviderExchanges))
				}
				first, second := record.ProviderExchanges[0], record.ProviderExchanges[1]
				if first.Attempt != 1 || first.Protocol != routeCase.firstProtocol || first.Outcome != requestrecord.OutcomeFailed {
					t.Fatalf("first exchange = attempt %d protocol %q outcome %q", first.Attempt, first.Protocol, first.Outcome)
				}
				if second.Attempt != 2 || second.Protocol != routeCase.secondProtocol || second.Outcome != requestrecord.OutcomeSucceeded {
					t.Fatalf("second exchange = attempt %d protocol %q outcome %q", second.Attempt, second.Protocol, second.Outcome)
				}
				if record.FinalResponse == nil || record.FinalResponse.Protocol != routeCase.source {
					t.Fatalf("final response = %#v, want %s", record.FinalResponse, routeCase.source)
				}
			})
		}
	}
}

func TestServerProtocolStageRecordingFailoverExhausted(t *testing.T) {
	recordDir := t.TempDir()
	env := NewTestEnv(t,
		NewTestEnvOptionWithProtocolStage(),
		NewTestEnvOptionWithRecordDir(recordDir),
	)
	route := env.SetupBothFailingRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, FailMockPreContent500)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)
	if result.HTTPStatus == http.StatusOK {
		t.Fatal("exhausted failover unexpectedly returned 200")
	}
	env.Close()

	records := readPersistedRequestRecords(t, recordDir)
	if len(records) != 1 {
		t.Fatalf("RequestRecord count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Outcome != requestrecord.OutcomeFailed {
		t.Fatalf("request outcome = %q, want failed", record.Outcome)
	}
	if len(record.ProviderExchanges) != 2 {
		t.Fatalf("provider exchange count = %d, want 2", len(record.ProviderExchanges))
	}
	for index, exchange := range record.ProviderExchanges {
		if exchange.Attempt != index+1 || exchange.Outcome != requestrecord.OutcomeFailed {
			t.Fatalf("exchange %d = attempt %d outcome %q", index, exchange.Attempt, exchange.Outcome)
		}
	}
	if record.FinalResponse != nil {
		t.Fatalf("final response = %#v, want nil", record.FinalResponse)
	}
}

func readPersistedRequestRecords(t *testing.T, root string) []*requestrecord.RequestRecord {
	t.Helper()
	var records []*requestrecord.RequestRecord
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".jsonl.gz") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		reader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer reader.Close()
		decoder := json.NewDecoder(reader)
		for {
			var envelope struct {
				RequestRecord *requestrecord.RequestRecord `json:"request_record"`
			}
			if err := decoder.Decode(&envelope); err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if envelope.RequestRecord != nil {
				records = append(records, envelope.RequestRecord)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read persisted RequestRecords: %v", err)
	}
	return records
}

func TestServerProtocolStageAnthropicBetaGuardrailComplete(t *testing.T) {
	t.Parallel()

	for _, target := range []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses} {
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

	for _, target := range []protocol.APIType{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			t.Parallel()
			runtime := newProtocolStageGuardrails(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
				if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil {
					return protocolStageBlockedResult("command denied"), nil
				}
				return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
			})
			env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithGuardrails(runtime))
			scenario := StreamingToolUseScenario()
			env.SetupRoute(protocol.TypeAnthropicBeta, target, scenario)
			model := env.findRouteModel(protocol.TypeAnthropicBeta, target, scenario.Name)
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
		})
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

	for _, target := range []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat} {
		target := target
		for _, streaming := range []bool{false, true} {
			streaming := streaming
			name := string(target) + "/nonstream"
			if streaming {
				name = string(target) + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				env := NewTestEnv(t, NewTestEnvOptionWithProtocolStage())
				scenario := TextScenario()
				model := env.SetupRouteWithFlags(
					protocol.TypeOpenAIChat,
					target,
					scenario,
					typ.RuleFlags{SkipUsage: true},
				)
				path, body := buildRequest(protocol.TypeOpenAIChat, model, streaming)
				result, err := env.dispatch(
					protocol.TypeOpenAIChat,
					target,
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
