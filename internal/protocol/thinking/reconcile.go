package thinking

import (
	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
)

// ReconcileV1WithRequest makes the request's thinking legal on the Anthropic
// wire given the rest of the request. Callers decide where it runs (the
// OpenAI → Anthropic conversions and the Claude Code vendor path):
//   - a tool-use turn with no thinking block turns budget thinking off
//     (DisableV1ForUnsignedToolTurn);
//   - forced tool use (tool_choice any / tool) turns thinking off, since
//     Anthropic rejects it with thinking and the tool call is what the client
//     needs;
//   - otherwise the sampling parameters thinking forbids are dropped:
//     temperature other than 1 and top_k; top_p is raised to 0.95.
func ReconcileV1WithRequest(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	DisableV1ForUnsignedToolTurn(req)
	if req.Thinking.OfEnabled == nil && req.Thinking.OfAdaptive == nil {
		return
	}
	if req.ToolChoice.OfAny != nil || req.ToolChoice.OfTool != nil {
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
		return
	}
	sampling := reconcileSampling(req.ExtraFields())
	req.SetExtraFields(sampling)
}

// ReconcileBetaWithRequest is ReconcileV1WithRequest for Beta
// requests.
func ReconcileBetaWithRequest(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	DisableBetaForUnsignedToolTurn(req)
	if req.Thinking.OfEnabled == nil && req.Thinking.OfAdaptive == nil {
		return
	}
	if req.ToolChoice.OfAny != nil || req.ToolChoice.OfTool != nil {
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
		return
	}
	sampling := reconcileSampling(req.ExtraFields())
	req.SetExtraFields(sampling)
}

// minThinkingTopP is the smallest top_p Anthropic accepts with thinking on.
const minThinkingTopP = 0.95

// reconcileSampling applies thinkingSampling to the request's sampling
// extras. v2 SDK params no longer have temperature/top_p/top_k fields; with
// the pinned 2023-06-01 API version the server still accepts them, so they
// travel as extra fields. Dropping a value sets param.Omit so a decoded
// inbound value is not re-sent.
func reconcileSampling(extras map[string]any) map[string]any {
	if len(extras) == 0 {
		return nil
	}
	out := make(map[string]any, 3)
	if v, ok := extras["temperature"]; ok {
		if f, ok := v.(float64); !ok || f == 1 {
			out["temperature"] = v
		} else {
			out["temperature"] = param.Omit
		}
	}
	if _, ok := extras["top_k"]; ok {
		out["top_k"] = param.Omit
	}
	if v, ok := extras["top_p"]; ok {
		if f, ok := v.(float64); ok && f < minThinkingTopP {
			out["top_p"] = minThinkingTopP
		} else {
			out["top_p"] = v
		}
	}
	return out
}

// DisableV1ForUnsignedToolTurn turns budget thinking off when the
// request continues a tool-use turn whose assistant message carries no
// thinking block. With budget thinking enabled, Anthropic requires that final
// assistant message to start with a (signed) thinking block, and a client that
// never sees thinking blocks — an OpenAI client, or a rule forcing
// thinking_effort onto one — has none to send back. Run it before any
// model-specific thinking-block filtering, on the client's own history; a
// model with mandatory thinking then turns "disabled" into adaptive.
func DisableV1ForUnsignedToolTurn(req *anthropic.MessageNewParams) {
	if req != nil && req.Thinking.OfEnabled != nil && toolTurnWithoutThinking(req.Messages) {
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
	}
}

// DisableBetaForUnsignedToolTurn is DisableV1ForUnsignedToolTurn
// for Beta requests.
func DisableBetaForUnsignedToolTurn(req *anthropic.BetaMessageNewParams) {
	if req != nil && req.Thinking.OfEnabled != nil && betaToolTurnWithoutThinking(req.Messages) {
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
	}
}

// toolTurnWithoutThinking reports whether the last assistant message calls a
// tool without starting with a thinking block.
func toolTurnWithoutThinking(messages []anthropic.MessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if msgparam.Role(messages[i]) != "assistant" {
			continue
		}
		content := msgparam.Content(messages[i])
		hasToolUse := false
		for _, block := range content {
			if block.OfToolUse != nil {
				hasToolUse = true
			}
		}
		return hasToolUse && content[0].OfThinking == nil && content[0].OfRedactedThinking == nil
	}
	return false
}

// betaToolTurnWithoutThinking is toolTurnWithoutThinking for Beta messages.
func betaToolTurnWithoutThinking(messages []anthropic.BetaMessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if msgparam.BetaRole(messages[i]) != "assistant" {
			continue
		}
		content := msgparam.BetaContent(messages[i])
		hasToolUse := false
		for _, block := range content {
			if block.OfToolUse != nil {
				hasToolUse = true
			}
		}
		return hasToolUse && content[0].OfThinking == nil && content[0].OfRedactedThinking == nil
	}
	return false
}
