package protocoltest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
)

// Guardrails x MCP through the Tool Round Stage, in memory, with the real
// Guardrails pipelines behind the gate. Each test names the gap it covers;
// the HTTP harness keeps those gaps registered until the cutover wires the
// stage in.

const (
	toolRoundSecret = "sk-toolround-secret"
	toolRoundAlias  = "TINGLY_CRED_TOKEN_TOOLROUND"
)

func toolRoundGate(runtime *guardrails.Guardrails) toolround.Gate {
	return guardrailspipeline.NewToolRoundGate(runtime, guardrailscore.Input{Scenario: "anthropic", Model: "m"})
}

// withCredential installs one protected credential for the anthropic scenario.
func withCredential(runtime *guardrails.Guardrails, secret string) *guardrails.Guardrails {
	runtime.SetCredentialCache(guardrails.BuildCredentialCache([]guardrailscore.ProtectedCredential{{
		ID: "toolround-credential", Name: "toolround credential",
		Type: guardrailscore.ProtectedCredentialTypeToken, Secret: secret, AliasToken: toolRoundAlias, Enabled: true,
	}}, []string{"anthropic"}))
	return runtime
}

func allowAllGuardrails() *guardrails.Guardrails {
	return &guardrails.Guardrails{
		Policy: guardrailsPolicyFunc(func(context.Context, guardrailscore.Input) (guardrailscore.Result, error) {
			return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
		}),
		HasActivePolicies: true,
	}
}

// G2: a server-owned tool blocked by Guardrails never runs.
func TestToolRoundGuardrailsBlocksServerToolInMemory(t *testing.T) {
	forToolRoundTargets(t, allToolRoundTargets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newGatedToolRoundHarness(t, NewEchoServertoolProvider(), toolRoundGate(newBlockToolNamedGuardrails("echo")), target, OwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Empty(t, h.echo.Calls(), "a blocked server tool never runs")
		require.Len(t, h.terminal.requests, 1)
		require.Contains(t, out, "Blocked by guardrails")
		require.NotContains(t, out, ownedToolUseMarker, "the blocked call itself never reaches the client")
	})
}

// ownedToolUseMarker appears only in a tool_use block for the server tool;
// block messages may still name the command they refused.
const ownedToolUseMarker = `"name":"` + OwnedToolWireName + `"`

// G8 and composition order: the server tool runs, and the client tool the
// model calls next is blocked before it reaches the client - on every target.
func TestToolRoundGuardrailsBlocksClientToolAfterServerRoundInMemory(t *testing.T) {
	targets := []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
	forToolRoundTargets(t, targets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newGatedToolRoundHarness(t, NewEchoServertoolProvider(), toolRoundGate(newBlockToolNamedGuardrails(clientToolName)), target, OwnedThenClientToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Len(t, h.echo.Calls(), 1)
		require.Contains(t, out, "Blocked by guardrails")
		require.NotContains(t, out, `"name":"`+clientToolName+`"`, "the blocked client tool never reaches the client")
		require.NotContains(t, out, ownedToolUseMarker)
	})
}

// G5 and G7: every call of a round is evaluated, each round against the
// conversation as it stands.
func TestToolRoundGuardrailsEvaluatesEveryCallWithCurrentHistoryInMemory(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{} // tool name -> history length at evaluation
	runtime := &guardrails.Guardrails{
		Policy: guardrailsPolicyFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
			if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil {
				mu.Lock()
				seen[input.Content.Command.Name] = len(input.Content.Messages)
				mu.Unlock()
			}
			return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
		}),
		HasActivePolicies: true,
	}
	h := newGatedToolRoundHarness(t, NewEchoServertoolProvider(), toolRoundGate(runtime), protocol.TypeAnthropicBeta, OwnedThenClientToolScenario())
	_, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), false)
	require.NoError(t, err)
	require.Contains(t, seen, OwnedToolWireName, "server tools are evaluated too")
	require.Contains(t, seen, clientToolName)
	require.Greater(t, seen[clientToolName], seen[OwnedToolWireName], "later rounds see the server round in their history")

	seen = map[string]int{}
	h = newGatedToolRoundHarness(t, NewEchoServertoolProvider(), toolRoundGate(runtime), protocol.TypeAnthropicBeta, MixedToolScenario())
	_, err = h.send(context.Background(), t, offeredRequest(t, firstTurn), true)
	require.NoError(t, err)
	require.Contains(t, seen, OwnedToolWireName, "every call of a round is evaluated, not only the first")
	require.Contains(t, seen, clientToolName)
}

// G3: a server tool's result is masked before the model sees it.
func TestToolRoundGuardrailsMasksServerToolResultInMemory(t *testing.T) {
	forToolRoundTargets(t, []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}, func(t *testing.T, target protocol.APIType, streaming bool) {
		gate := toolRoundGate(withCredential(allowAllGuardrails(), ownedToolResultText))
		h := newGatedToolRoundHarness(t, NewEchoServertoolProvider(), gate, target, OwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Len(t, h.echo.Calls(), 1, "requests=%v\nout=%s", h.terminal.requests, out)
		require.Contains(t, h.lastRequest(), toolRoundAlias)
		require.NotContains(t, h.lastRequest(), ownedToolResultText, "the protected value never reaches the model")
	})
}

// G3: a server tool's result that Guardrails blocks is replaced before the
// model sees it; the conversation around it is untouched.
func TestToolRoundGuardrailsBlocksServerToolResultInMemory(t *testing.T) {
	runtime := &guardrails.Guardrails{
		Policy: guardrailsPolicyFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
			if input.Direction == guardrailscore.DirectionRequest && input.HasToolResult && strings.Contains(input.Content.Text, ownedToolResultText) {
				return guardrailscore.Result{
					Verdict: guardrailscore.VerdictBlock,
					Reasons: []guardrailscore.PolicyResult{{PolicyID: "toolround-block-result", Verdict: guardrailscore.VerdictBlock, Reason: "result denied"}},
				}, nil
			}
			return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
		}),
		HasActivePolicies: true,
	}
	both := []protocol.APIType{protocol.TypeAnthropicBeta}
	forToolRoundTargets(t, both, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newGatedToolRoundHarness(t, NewEchoServertoolProvider(), toolRoundGate(runtime), target, OwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.NotContains(t, h.lastRequest(), ownedToolResultText)
		require.Contains(t, h.lastRequest(), "Blocked by guardrails")
		require.Contains(t, h.lastRequest(), "What is the capital of France?", "the rest of the conversation is untouched")
		require.Contains(t, out, OwnedToolFinalText)
	})
}

// G4: an alias the model writes into a server tool's input is restored to the
// real value before the tool runs.
func TestToolRoundGuardrailsRestoresAliasForServerToolInMemory(t *testing.T) {
	both := []protocol.APIType{protocol.TypeAnthropicBeta}
	forToolRoundTargets(t, both, func(t *testing.T, target protocol.APIType, streaming bool) {
		echo := NewEchoServertoolProvider()
		gate := toolRoundGate(withCredential(allowAllGuardrails(), toolRoundSecret))
		h := newGatedToolRoundHarness(t, echo, gate, target, aliasEchoScenario(toolRoundAlias))
		prompt := fmt.Sprintf(`[{"role":"user","content":"use key %s"}]`, toolRoundSecret)
		_, err := h.send(context.Background(), t, offeredRequest(t, prompt), streaming)
		require.NoError(t, err)
		require.NotContains(t, h.terminal.requests[0], toolRoundSecret, "the client's secret is masked upstream")
		require.Len(t, echo.Calls(), 1)
		require.Equal(t, toolRoundSecret, echo.Calls()[0]["q"], "the tool runs on the real value")
	})
}

// aliasEchoScenario is OwnedToolScenario on Anthropic with the model passing
// alias as the echo tool's argument.
func aliasEchoScenario(alias string) Scenario {
	base := OwnedToolScenario()
	first := matrixAnthropicOwnedTool()
	first["content"] = []map[string]any{{"type": "tool_use", "id": "toolu-owned-tool", "name": OwnedToolWireName, "input": map[string]any{"q": alias}}}
	var firstStream []string
	for _, line := range matrixAnthropicOwnedToolStream() {
		firstStream = append(firstStream, strings.ReplaceAll(line, `{\"q\":\"x\"}`, `{\"q\":\"`+alias+`\"}`))
	}
	base.MockResponses = map[ResponseFormat]MockResponseBuilder{
		FormatAnthropic: {
			NonStreamFor: func(request []byte) (int, []byte) {
				if callsOwnedTool(request) {
					return http.StatusOK, mustMarshal(first)
				}
				return http.StatusOK, mustMarshal(matrixAnthropicOwnedToolFinal())
			},
			StreamFor: func(request []byte) []string {
				if callsOwnedTool(request) {
					return firstStream
				}
				return matrixAnthropicOwnedToolFinalStream()
			},
		},
	}
	return base
}
