package request

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// Budget-to-effort tiers. Anthropic's budget_tokens is a token count;
// Responses' reasoning.effort is a coarse enum. Both DeepSeek and OpenAI
// accept the full effort range, so one mapping serves both vendors.
const (
	reasoningEffortLowMaxBudget    = 4096
	reasoningEffortMediumMaxBudget = 16384
)

// reasoningEffortForBudget maps an Anthropic thinking budget to a Responses
// reasoning effort tier. Below reasoningEffortLowMaxBudget tokens maps to
// low, below reasoningEffortMediumMaxBudget to medium, everything else to
// high — there's no lossless mapping from a token count to an enum, so this
// picks the tier whose typical reasoning depth is closest.
func reasoningEffortForBudget(budgetTokens int64) shared.ReasoningEffort {
	switch {
	case budgetTokens < reasoningEffortLowMaxBudget:
		return shared.ReasoningEffortLow
	case budgetTokens < reasoningEffortMediumMaxBudget:
		return shared.ReasoningEffortMedium
	default:
		return shared.ReasoningEffortHigh
	}
}

// anthropicV1ThinkingToReasoning converts a v1 request's thinking config to
// a Responses reasoning param. Returns ok=false when thinking isn't enabled
// (disabled, adaptive, or omitted) — callers leave params.Reasoning unset.
func anthropicV1ThinkingToReasoning(thinking anthropic.ThinkingConfigParamUnion) (shared.ReasoningParam, bool) {
	if thinking.OfEnabled == nil {
		return shared.ReasoningParam{}, false
	}
	return shared.ReasoningParam{Effort: reasoningEffortForBudget(thinking.OfEnabled.BudgetTokens)}, true
}

// anthropicBetaThinkingToReasoning is the beta-request equivalent of
// anthropicV1ThinkingToReasoning.
func anthropicBetaThinkingToReasoning(thinking anthropic.BetaThinkingConfigParamUnion) (shared.ReasoningParam, bool) {
	if thinking.OfEnabled == nil {
		return shared.ReasoningParam{}, false
	}
	return shared.ReasoningParam{Effort: reasoningEffortForBudget(thinking.OfEnabled.BudgetTokens)}, true
}

// newSyntheticReasoningItemID generates an id for a reasoning input item
// reconstructed from Anthropic history. Responses requires an id on
// ResponseReasoningItemParam, but Anthropic thinking blocks carry no
// upstream-issued id (Signature is an opaque continuity token, not one) —
// this is a synthetic, request-scoped identifier good only for this replay.
func newSyntheticReasoningItemID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "rs_local"
	}
	return "rs_" + hex.EncodeToString(b)
}

// reasoningInputItemFromThinkingText converts one Anthropic thinking block's
// text into a Responses `reasoning` input item, so history replay carries
// the model's prior reasoning forward the same way a text or tool_use block
// does. An empty thinking text produces no item (nothing to carry).
func reasoningInputItemFromThinkingText(thinkingText string) *responses.ResponseInputItemUnionParam {
	if thinkingText == "" {
		return nil
	}
	item := responses.ResponseInputItemParamOfReasoning(
		newSyntheticReasoningItemID(),
		[]responses.ResponseReasoningItemSummaryParam{{Text: thinkingText}},
	)
	return &item
}
