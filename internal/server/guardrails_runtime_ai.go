package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file holds the gateway-facing half of guardrails runtime evaluation:
// building the evaluation envelope and applying request/response mutation
// during a live model request. The other half — the mutex-guarded runtime
// pointer itself and its admin-facing accessors (swap, activation refresh,
// credential cache refresh) — stays in root server/guardrails_runtime.go as
// the single source of truth, since webui's admin handlers and root's own
// lifecycle code (NewServer, config watcher, options) need it too.
//
// Callers here receive the current *guardrails.Guardrails snapshot as an
// explicit parameter (typically via (*server.Server).CurrentGuardrailsRuntime())
// rather than reaching for shared state, keeping this package independent of
// *server.Server.

// GuardrailsSupportedScenarios lists the scenarios guardrails can gate.
var GuardrailsSupportedScenarios = []string{
	string(typ.ScenarioAnthropic),
	string(typ.ScenarioClaudeCode),
}

// GuardrailsSupportsScenario reports whether scenario is one guardrails can gate.
func GuardrailsSupportsScenario(scenario string) bool {
	base := typ.RuleScenario(scenario).Base()
	for _, supported := range GuardrailsSupportedScenarios {
		if base == typ.RuleScenario(supported) {
			return true
		}
	}
	return false
}

// GuardrailsEnabledForScenario centralizes feature-flag checks so protocol
// handlers do not repeat scenario/global guardrails gating logic.
func GuardrailsEnabledForScenario(cfg *config.Config, runtime *guardrails.Guardrails, scenario string) bool {
	if runtime == nil || runtime.PolicyEngine() == nil || cfg == nil || !runtime.IsActive() {
		return false
	}
	if !GuardrailsSupportsScenario(scenario) {
		return false
	}
	return cfg.GetScenarioFlag(typ.RuleScenario(scenario), config.ExtensionGuardrails) ||
		cfg.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionGuardrails)
}

func EnsureGuardrailsCredentialMaskState(c *gin.Context) *guardrailscore.CredentialMaskState {
	if c == nil {
		return nil
	}
	if existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrailscore.CredentialMaskState); ok && state != nil {
			return state
		}
	}
	state := guardrailscore.NewCredentialMaskState()
	c.Set(guardrailscore.CredentialMaskStateContextKey, state)
	return state
}

// ----------------------------------------------------------------------
// Shared Input Construction
// ----------------------------------------------------------------------

// BuildGuardrailsBaseInput creates the shared evaluation envelope; adapters can then
// add request/response-specific content without rebuilding metadata each time.
func BuildGuardrailsBaseInput(c *gin.Context, actualModel string, provider *typ.Provider, direction guardrailscore.Direction, messages []guardrailscore.Message) guardrailscore.Input {
	_, _, _, requestModel, scenario, _, _ := GetTrackingContext(c)
	providerName := ""
	if provider != nil {
		providerName = provider.Name
	}
	return guardrailscore.Input{
		Scenario:  scenario,
		Model:     actualModel,
		Direction: direction,
		Content: guardrailscore.Content{
			Messages: messages,
		},
		Metadata: map[string]interface{}{
			"provider":      providerName,
			"request_model": requestModel,
		},
		Runtime: guardrailscore.InputRuntime{
			Context: c,
		},
	}
}

// ----------------------------------------------------------------------
// Stream Response Guardrails
// ----------------------------------------------------------------------

// AttachGuardrailsHooks wires the shared stream guardrails runtime into a protocol
// handle context. Provider-specific handlers only need to provide already-normalized
// message history.
func AttachGuardrailsHooks(c *gin.Context, runtime *guardrails.Guardrails, hc *protocol.HandleContext, actualModel string, provider *typ.Provider, messages []guardrailscore.Message) {
	guardrailsState := hc.EnsureGuardrails()
	baseInput := BuildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messages)
	maskState := EnsureGuardrailsCredentialMaskState(c)
	baseInput.State.CredentialMask = maskState
	guardrailsState.Enabled = true
	guardrailsState.CredentialMask = baseInput.State.CredentialMask
	streamState := hc.EnsureGuardrailsStream()
	logrus.Debugf("Guardrails: attaching hook (scenario=%s model=%s)", baseInput.Scenario, baseInput.Model)

	onEvent, onError := guardrailspipeline.NewGuardrailsHooks(
		c.Request.Context(),
		runtime,
		baseInput,
		streamState,
	)
	if onEvent != nil {
		hc.WithOnStreamEvent(onEvent)
	}
	if onError != nil {
		hc.WithOnStreamError(onError)
	}
}

// ReattachGuardrailsHooks resets per-round guardrails state and re-registers fresh
// hooks on hc for the next MCP loop round. It truncates OnStreamEventHooks back to
// baseEventHooks (the count before guardrails was first attached) so previous-round
// guardrails hooks don't accumulate.
func ReattachGuardrailsHooks(
	c *gin.Context,
	runtime *guardrails.Guardrails,
	hc *protocol.HandleContext,
	actualModel string,
	provider *typ.Provider,
	messages []guardrailscore.Message,
	baseEventHooks int,
	baseErrorHooks int,
) {
	// Truncate back to pre-guardrails hooks
	if len(hc.OnStreamEventHooks) > baseEventHooks {
		hc.OnStreamEventHooks = hc.OnStreamEventHooks[:baseEventHooks]
	}
	if len(hc.OnStreamErrorHooks) > baseErrorHooks {
		hc.OnStreamErrorHooks = hc.OnStreamErrorHooks[:baseErrorHooks]
	}
	// Reset stream accumulator state so the new round starts clean
	if hc.Guardrails != nil {
		hc.Guardrails.Stream = nil
	}
	// Re-attach with a fresh accumulator
	AttachGuardrailsHooks(c, runtime, hc, actualModel, provider, messages)
}

// ----------------------------------------------------------------------
// Request Guardrails
// ----------------------------------------------------------------------

// ApplyGuardrailsToAnthropicV1Request is the merged request-side entry for
// Anthropic v1 requests. It runs request tool_result filtering first and then
// request credential masking on the latest raw request state.
func ApplyGuardrailsToAnthropicV1Request(c *gin.Context, runtime *guardrails.Guardrails, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}

	input := BuildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionRequest, nil)
	input.State.CredentialMask = EnsureGuardrailsCredentialMaskState(c)
	input.Payload.Protocol = "anthropic_v1"
	input.Payload.Request = req

	err := guardrailspipeline.ProcessAnthropicV1Request(
		c.Request.Context(),
		runtime,
		input,
	)
	if err != nil {
		return
	}
}

// ApplyGuardrailsToAnthropicV1BetaRequest is the merged request-side entry for
// Anthropic beta requests. It runs request tool_result filtering first and then
// request credential masking on the latest raw request state.
func ApplyGuardrailsToAnthropicV1BetaRequest(c *gin.Context, runtime *guardrails.Guardrails, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}

	input := BuildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionRequest, nil)
	input.State.CredentialMask = EnsureGuardrailsCredentialMaskState(c)
	input.Payload.Protocol = "anthropic_beta"
	input.Payload.Request = req

	err := guardrailspipeline.ProcessAnthropicBetaRequest(
		c.Request.Context(),
		runtime,
		input,
	)
	if err != nil {
		return
	}
}

// ----------------------------------------------------------------------
// Non-Stream Response Guardrails
// ----------------------------------------------------------------------

// ApplyGuardrailsToAnthropicV1NonStreamResponse evaluates a fully assembled
// Anthropic v1 response and rewrites it when guardrails block it.
func ApplyGuardrailsToAnthropicV1NonStreamResponse(c *gin.Context, runtime *guardrails.Guardrails, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider, resp *anthropic.Message) bool {
	if req == nil || resp == nil {
		return false
	}

	maskState := EnsureGuardrailsCredentialMaskState(c)
	messageHistory := guardrailsadapter.AdaptMessagesFromAnthropicV1(req.System, req.Messages)
	input := BuildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.State.CredentialMask = maskState
	input.Payload.Protocol = "anthropic_v1"
	input.Payload.Response = resp

	mutation, err := guardrailspipeline.ProcessAnthropicV1NonStreamResponse(c.Request.Context(), runtime, input, resp)
	if err != nil {
		return false
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1ResponseCredentials(maskState, resp)
	}
	return mutation.Changed
}

// ApplyGuardrailsToAnthropicV1BetaNonStreamResponse is the beta equivalent of
// ApplyGuardrailsToAnthropicV1NonStreamResponse.
func ApplyGuardrailsToAnthropicV1BetaNonStreamResponse(c *gin.Context, runtime *guardrails.Guardrails, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider, resp *anthropic.BetaMessage) bool {
	if req == nil || resp == nil {
		return false
	}

	maskState := EnsureGuardrailsCredentialMaskState(c)
	messageHistory := guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages)
	input := BuildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.State.CredentialMask = maskState
	input.Payload.Protocol = "anthropic_beta"
	input.Payload.Response = resp

	mutation, err := guardrailspipeline.ProcessAnthropicV1BetaNonStreamResponse(c.Request.Context(), runtime, input, resp)
	if err != nil {
		return false
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1BetaResponseCredentials(maskState, resp)
	}
	return mutation.Changed
}
