package protocoltest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

var _ = registerKnownGaps(KnownGap{
	ID:     "M1",
	Reason: "Responses source: the server tool is injected upstream but its call is not intercepted, so it leaks to the client",
},
	"TestMCPOwnedToolLoop/openai_responses->anthropic_beta/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->anthropic_beta/stream=true",
	"TestMCPOwnedToolLoop/openai_responses->openai_chat/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->openai_chat/stream=true",
) && registerKnownGaps(KnownGap{
	ID:     "M2",
	Reason: "Chat->Chat streaming tool loop: the tool runs but the final answer reaches the client as empty data frames",
},
	"TestMCPOwnedToolLoop/openai_chat->openai_chat/stream=true",
) && registerKnownGaps(KnownGap{
	ID:     "M3",
	Reason: "OpenAI Responses target: server tools are not offered to the model at all",
},
	"TestMCPOwnedToolLoop/anthropic_v1->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/anthropic_v1->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/anthropic_beta->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/anthropic_beta->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/openai_chat->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/openai_chat->openai_responses/stream=true",
	"TestMCPOwnedToolLoop/openai_responses->openai_responses/stream=false",
	"TestMCPOwnedToolLoop/openai_responses->openai_responses/stream=true",
)

// TestMCPOwnedToolLoop pins the server-owned tool loop through the real HTTP
// gateway for every protocol pair: the gateway offers its tool, executes the
// model's call exactly once with the model's arguments, continues the
// conversation, and returns only the final answer — the client never sees the
// server tool.
func TestMCPOwnedToolLoop(t *testing.T) {
	t.Parallel()

	for _, pair := range DefaultPairs() {
		for _, streaming := range []bool{false, true} {
			pair, streaming := pair, streaming
			name := fmt.Sprintf("%s->%s/stream=%v", pair.Source, pair.Target, streaming)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				echo := NewEchoServertoolProvider()
				env := NewTestEnv(t, NewTestEnvOptionWithServertoolProviders(echo))
				scenario := OwnedToolScenario()
				env.SetupRoute(pair.Source, pair.Target, scenario)
				model := env.findRouteModel(pair.Source, pair.Target, scenario.Name)
				path, body := buildRequest(pair.Source, model, streaming)

				status, raw := sendRaw(t, env, path, body)

				var failures []string
				if status != 200 {
					failures = append(failures, fmt.Sprintf("status = %d", status))
				}
				calls := echo.Calls()
				if len(calls) != 1 {
					failures = append(failures, fmt.Sprintf("server tool executed %d times, want 1", len(calls)))
				} else if calls[0]["q"] != "x" {
					failures = append(failures, fmt.Sprintf("server tool args = %v, want q=x", calls[0]))
				}
				if got := env.VirtualCallCount(); got != 2 {
					failures = append(failures, fmt.Sprintf("upstream calls = %d, want 2 (tool round + final round)", got))
				}
				if !strings.Contains(raw, OwnedToolFinalText) {
					failures = append(failures, "final answer missing from client response")
				}
				if strings.Contains(raw, OwnedToolWireName) {
					failures = append(failures, "server tool call leaked to client")
				}
				checkCase(t, t.Name(), failures, "client response:\n"+raw)
			})
		}
	}
}

// TestMCPOwnedToolNotOfferedWhenDisabled pins that without the MCP extension
// the gateway neither offers nor executes server tools.
func TestMCPOwnedToolNotOfferedWhenDisabled(t *testing.T) {
	t.Parallel()

	env := NewTestEnv(t)
	scenario := OwnedToolScenario()
	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, scenario)
	model := env.findRouteModel(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, scenario.Name)
	path, body := buildRequest(protocol.TypeAnthropicBeta, model, false)

	status, raw := sendRaw(t, env, path, body)
	if status != 200 {
		t.Fatalf("status = %d: %s", status, raw)
	}
	if upstream := string(env.virtual.LastRequest(EndpointAnthropic).Body); strings.Contains(upstream, OwnedToolWireName) {
		t.Fatalf("server tool offered upstream with MCP disabled:\n%s", upstream)
	}
	if got := env.VirtualCallCount(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}
