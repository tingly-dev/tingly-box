package protocoltest

import (
	"fmt"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestAnthropicClientThinkingReachesUpstreamAsSent guards the prompt cache: a
// provider's cached prefix is invalidated when the request's thinking setting
// changes, so an Anthropic client's thinking must not be rewritten from one
// request to the next. The gateway used to switch it off on any tool-loop turn
// whose last assistant message had no thinking block — which providers that do
// not always return one leave behind — and back on at the next user turn.
func TestAnthropicClientThinkingReachesUpstreamAsSent(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(streamMode(streaming), func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			s := cachePrefixScenario()
			source, target := protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta
			env.SetupRoute(source, target, s)
			model := env.findRouteModel(source, target, s.Name)

			// Two requests as one Claude Code session sends them: the tool-loop
			// turn (history ends at a tool_result whose assistant message has no
			// thinking block), and the turn after the user speaks again.
			toolTurn := anthropicCachePrefixBody(model, streaming, 2, -1, cachePrefixSessionID, true)
			userTurn := anthropicCachePrefixBody(model, streaming, 2, -1, cachePrefixSessionID, true)
			userTurn["messages"] = append(userTurn["messages"].([]map[string]any), map[string]any{
				"role": "assistant", "content": []map[string]any{{"type": "text", "text": "done"}},
			}, map[string]any{
				"role": "user", "content": []map[string]any{{"type": "text", "text": "again"}},
			})
			for _, body := range []map[string]any{toolTurn, userTurn} {
				body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 1024}
				body["max_tokens"] = 4096
			}

			var seen []string
			for name, body := range map[string]map[string]any{"tool_turn": toolTurn, "user_turn": userTurn} {
				path, _ := buildRequest(source, model, streaming)
				if _, err := env.dispatch(source, target, s.Name, path, mustMarshal(body), nil, streaming); err != nil {
					t.Fatalf("%s: dispatch: %v", name, err)
				}
				upstream, _ := requireLastRequest(t, env, target, name).JSON()["thinking"].(map[string]any)
				got, _ := upstream["type"].(string)
				seen = append(seen, fmt.Sprintf("%s=%s", name, got))
				if got != "enabled" {
					t.Errorf("%s: upstream thinking.type = %q, want the client's \"enabled\" — a changed thinking setting invalidates the provider's cache", name, got)
				}
			}
			t.Logf("upstream thinking per request: %v", seen)
		})
	}
}
