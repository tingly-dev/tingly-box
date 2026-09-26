package protocoltest

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// guardrailsPolicyFunc adapts a function into a guardrails policy so a test
// can decide verdicts from the evaluated input.
type guardrailsPolicyFunc func(context.Context, guardrailscore.Input) (guardrailscore.Result, error)

func (f guardrailsPolicyFunc) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
	return f(ctx, input)
}

// newBlockToolUseGuardrails returns an active runtime that blocks every
// response-side tool call and allows everything else.
func newBlockToolUseGuardrails() *guardrails.Guardrails {
	return &guardrails.Guardrails{
		Policy: guardrailsPolicyFunc(func(_ context.Context, input guardrailscore.Input) (guardrailscore.Result, error) {
			if input.Direction == guardrailscore.DirectionResponse && input.Content.Command != nil {
				return guardrailscore.Result{
					Verdict: guardrailscore.VerdictBlock,
					Reasons: []guardrailscore.PolicyResult{{
						PolicyID: "protocoltest-block-tool-use",
						Verdict:  guardrailscore.VerdictBlock,
						Reason:   "tool use denied by test policy",
					}},
				}, nil
			}
			return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
		}),
		HasActivePolicies: true,
	}
}

// TestGuardrailsBlocksToolUseAnthropic pins that a blocked response tool_use
// never reaches an Anthropic client, on both the V1 path (served by the
// toolengine stream interceptor even without MCP) and the Beta passthrough.
func TestGuardrailsBlocksToolUseAnthropic(t *testing.T) {
	t.Parallel()

	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		for _, streaming := range []bool{false, true} {
			source, streaming := source, streaming
			t.Run(fmt.Sprintf("%s/stream=%v", source, streaming), func(t *testing.T) {
				t.Parallel()

				env := NewTestEnv(t, NewTestEnvOptionWithGuardrails(newBlockToolUseGuardrails()))
				scenario := ToolUseScenario()
				if streaming {
					scenario = StreamingToolUseScenario()
				}
				env.SetupRoute(source, source, scenario)
				model := env.findRouteModel(source, source, scenario.Name)
				path, body := buildRequest(source, model, streaming)

				status, raw := sendRaw(t, env, path, body)
				if status != http.StatusOK {
					t.Fatalf("status = %d: %s", status, raw)
				}
				if strings.Contains(raw, `"type":"tool_use"`) {
					t.Fatalf("blocked tool_use leaked to client:\n%s", raw)
				}
				if strings.Contains(raw, `"stop_reason":"tool_use"`) {
					t.Fatalf("stop_reason still tool_use after block:\n%s", raw)
				}
				if !strings.Contains(raw, "Blocked by guardrails") {
					t.Fatalf("response carries no block message:\n%s", raw)
				}
			})
		}
	}
}

// TestGuardrailsRestoresCredentialAliasAnthropic pins that a protected
// credential masked on the way upstream comes back to the client as the real
// value in a non-stream response, not as the alias token.
func TestGuardrailsRestoresCredentialAliasAnthropic(t *testing.T) {
	t.Parallel()

	const (
		secret = "sk-protocoltest-secret"
		alias  = "TINGLY_CRED_TOKEN_PROTOCOLTEST"
	)
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		source := source
		t.Run(string(source), func(t *testing.T) {
			t.Parallel()

			runtime := &guardrails.Guardrails{
				Policy: guardrailsPolicyFunc(func(context.Context, guardrailscore.Input) (guardrailscore.Result, error) {
					return guardrailscore.Result{Verdict: guardrailscore.VerdictAllow}, nil
				}),
				HasActivePolicies: true,
			}
			env := NewTestEnv(t, NewTestEnvOptionWithGuardrails(runtime))
			// Server boot refreshes the runtime's cache from the (empty) test
			// database, so install the credential on the live runtime afterwards.
			env.srv.CurrentGuardrailsRuntime().SetCredentialCache(guardrails.BuildCredentialCache(
				[]guardrailscore.ProtectedCredential{{
					ID: "protocoltest-credential", Name: "protocoltest credential",
					Type: guardrailscore.ProtectedCredentialTypeToken, Secret: secret, AliasToken: alias, Enabled: true,
				}},
				[]string{"anthropic"},
			))

			scenario := Scenario{
				Name: "credential_restore",
				MockResponses: map[ResponseFormat]MockResponseBuilder{
					FormatAnthropic: {
						NonStream: func() (int, []byte) {
							return http.StatusOK, mustMarshal(map[string]any{
								"id": "msg-credential", "type": "message", "role": "assistant",
								"content": []map[string]any{{"type": "text", "text": "key is " + alias}},
								"model":   "provider-model", "stop_reason": "end_turn", "stop_sequence": nil,
								"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
							})
						},
					},
				},
			}
			env.SetupRoute(source, source, scenario)
			model := env.findRouteModel(source, source, scenario.Name)
			path, body := buildRequest(source, model, false)
			// The alias is only registered once the request actually carries
			// the secret and request-side guardrails mask it.
			body = bytes.Replace(body, []byte("What is the capital of France?"), []byte("use "+secret), 1)

			status, raw := sendRaw(t, env, path, body)
			if status != http.StatusOK {
				t.Fatalf("status = %d: %s", status, raw)
			}
			if upstream := string(env.virtual.LastRequest(EndpointAnthropic).Body); strings.Contains(upstream, secret) {
				t.Fatalf("secret reached upstream unmasked:\n%s", upstream)
			}
			if strings.Contains(raw, alias) {
				t.Fatalf("credential alias leaked to client:\n%s", raw)
			}
			if !strings.Contains(raw, secret) {
				t.Fatalf("credential was not restored:\n%s", raw)
			}
		})
	}
}
