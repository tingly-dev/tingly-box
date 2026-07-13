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
		name       string
		opts       []TestEnvOption
		target     protocol.APIType
		streaming  bool
		wantHeader string
	}{
		{name: "default legacy", wantHeader: "legacy"},
		{name: "stage nonstream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, wantHeader: "stage"},
		{name: "stage stream", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, streaming: true, wantHeader: "stage"},
		{name: "stage unsupported pair stays legacy", opts: []TestEnvOption{NewTestEnvOptionWithProtocolStage()}, target: protocol.TypeOpenAIChat, wantHeader: "legacy"},
		{
			name:       "stage keeps MCP on legacy",
			opts:       []TestEnvOption{NewTestEnvOptionWithProtocolStage(), NewTestEnvOptionWithMCP()},
			wantHeader: "legacy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t, tt.opts...)
			scenario := TextScenario()
			target := tt.target
			if target == "" {
				target = protocol.TypeAnthropicBeta
			}
			env.SetupRoute(protocol.TypeOpenAIChat, target, scenario)
			model := env.findRouteModel(protocol.TypeOpenAIChat, target, scenario.Name)
			path, body := buildRequest(protocol.TypeOpenAIChat, model, tt.streaming)
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
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				t.Fatalf("read response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			if got := resp.Header.Get("X-Tingly-Protocol-Pipeline"); got != tt.wantHeader {
				t.Fatalf("pipeline header = %q, want %q", got, tt.wantHeader)
			}
			if got := resp.Header.Get("X-Tingly-Upstream-API"); got != string(target) {
				t.Fatalf("upstream API = %q", got)
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
