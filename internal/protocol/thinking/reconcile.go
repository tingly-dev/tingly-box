package thinking

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
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
	req.Temperature, req.TopK, req.TopP = thinkingSampling(req.Temperature, req.TopK, req.TopP)
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
	req.Temperature, req.TopK, req.TopP = thinkingSampling(req.Temperature, req.TopK, req.TopP)
}

// minThinkingTopP is the smallest top_p Anthropic accepts with thinking on.
const minThinkingTopP = 0.95

// thinkingSampling returns the sampling parameters thinking allows: no
// temperature other than 1, no top_k, and top_p within [0.95, 1].
func thinkingSampling(temperature param.Opt[float64], topK param.Opt[int64], topP param.Opt[float64]) (param.Opt[float64], param.Opt[int64], param.Opt[float64]) {
	if temperature.Valid() && temperature.Value != 1 {
		temperature = param.Opt[float64]{}
	}
	if topK.Valid() {
		topK = param.Opt[int64]{}
	}
	if topP.Valid() && topP.Value < minThinkingTopP {
		topP = param.NewOpt(minThinkingTopP)
	}
	return temperature, topK, topP
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
		if messages[i].Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		content := messages[i].Content
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
		if messages[i].Role != anthropic.BetaMessageParamRoleAssistant {
			continue
		}
		content := messages[i].Content
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
