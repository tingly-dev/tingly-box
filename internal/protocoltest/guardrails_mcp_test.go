package protocoltest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Guardrails x MCP composition. The intended order is one decision point per
// tool call: guardrails first, then ownership — a blocked server tool never
// runs, and a client tool produced after server rounds is still checked.
// Responses targets are left out: they are never offered server tools (M3).
var guardrailsMCPTargets = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}

// newBlockToolNamedGuardrails blocks response tool calls whose name contains
// fragment and allows everything else.
func newBlockToolNamedGuardrails(fragment string) *guardrails.Guardrails {
	return &guardrails.Guardrails{
		Policy: guardrailsPolicyFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
			if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil &&
				strings.Contains(input.Content.Command.Name, fragment) {
				return guardrailscore.Result{
					Verdict: guardrailscore.VerdictBlock,
					Reasons: []guardrailscore.PolicyResult{{PolicyID: "protocoltest-block-named", Verdict: guardrailscore.VerdictBlock, Reason: "denied by test policy"}},
				}, nil
			}
			return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
		}),
		HasActivePolicies: true,
	}
}

// TestGuardrailsBlocksServerTool pins that a server-owned tool call blocked
// by guardrails is not executed and the client gets the block message.
func TestGuardrailsBlocksServerTool(t *testing.T) {
	t.Parallel()

	for _, source := range anthropicSources {
		for _, target := range guardrailsMCPTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				t.Run(fmt.Sprintf("%s->%s/stream=%v", source, target, streaming), func(t *testing.T) {
					t.Parallel()

					echo := NewEchoServertoolProvider()
					env := NewTestEnv(t,
						NewTestEnvOptionWithServertoolProviders(echo),
						NewTestEnvOptionWithGuardrails(newBlockToolNamedGuardrails("echo")),
					)
					scenario := OwnedToolScenario()
					env.SetupRoute(source, target, scenario)
					model := env.findRouteModel(source, target, scenario.Name)
					path, body := buildRequest(source, model, streaming)

					status, raw := sendRaw(t, env, path, body)
					var failures []string
					if status != http.StatusOK {
						failures = append(failures, fmt.Sprintf("status = %d", status))
					}
					if n := len(echo.Calls()); n != 0 {
						failures = append(failures, fmt.Sprintf("blocked server tool executed %d times", n))
					}
					// The block message may name the refused command; only a
					// tool_use block for the server tool is a leak.
					if strings.Contains(raw, ownedToolUseMarker) {
						failures = append(failures, "server tool call leaked to client")
					}
					if !strings.Contains(raw, "Blocked by guardrails") {
						failures = append(failures, "no block message in response")
					}
					checkCase(t, t.Name(), failures, "client response:\n"+raw)
				})
			}
		}
	}
}

// TestGuardrailsBlocksClientToolAfterServerRound pins the composition order:
// the server tool runs (allowed), and the client tool the model calls in the
// next round is blocked before it reaches the client.
func TestGuardrailsBlocksClientToolAfterServerRound(t *testing.T) {
	t.Parallel()

	for _, source := range anthropicSources {
		for _, target := range guardrailsMCPTargets {
			for _, streaming := range []bool{false, true} {
				source, target, streaming := source, target, streaming
				t.Run(fmt.Sprintf("%s->%s/stream=%v", source, target, streaming), func(t *testing.T) {
					t.Parallel()

					echo := NewEchoServertoolProvider()
					env := NewTestEnv(t,
						NewTestEnvOptionWithServertoolProviders(echo),
						NewTestEnvOptionWithGuardrails(newBlockToolNamedGuardrails(clientToolName)),
					)
					scenario := OwnedThenClientToolScenario()
					env.SetupRoute(source, target, scenario)
					model := env.findRouteModel(source, target, scenario.Name)
					path, body := buildRequest(source, model, streaming)

					status, raw := sendRaw(t, env, path, body)
					failures := blockedToolUseFailures(status, raw)
					if n := len(echo.Calls()); n != 1 {
						failures = append(failures, fmt.Sprintf("allowed server tool executed %d times, want 1", n))
					}
					if strings.Contains(raw, clientToolName) && !strings.Contains(raw, "Blocked by guardrails") {
						failures = append(failures, "blocked client tool leaked to client")
					}
					checkCase(t, t.Name(), failures, "client response:\n"+raw)
				})
			}
		}
	}
}
