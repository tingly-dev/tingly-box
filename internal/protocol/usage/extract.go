// Package usage centralizes token extraction and normalization logic for all
// supported provider protocols. Every handler calls into this package instead
// of re-implementing provider-specific rules inline.
//
// Normalization rules:
//   - OpenAI (Chat / Responses): prompt_tokens = total (cached + uncached).
//     Store inputTokens = total - cached so the frontend ratio formula gives
//     cache_read / (cache_read + uncached) = correct hit rate.
//   - Anthropic: input_tokens = uncached only; cache_creation_input_tokens is
//     an additional write cost that belongs in the denominator.
//     Store inputTokens = input + creation so the formula covers total prompt cost.
package usage

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// FromOpenAIChatCompletion extracts normalized TokenUsage from an OpenAI Chat
// Completions usage block. CachedTokens is a SUBSET of PromptTokens, so we
// subtract it to get the uncached portion.
func FromOpenAIChatCompletion(u openai.CompletionUsage) *protocol.TokenUsage {
	cache := int(u.PromptTokensDetails.CachedTokens)
	reasoning := int(u.CompletionTokensDetails.ReasoningTokens)
	return protocol.NewTokenUsageFull(
		int(u.PromptTokens)-cache,
		int(u.CompletionTokens),
		cache,
		reasoning,
	)
}

// FromOpenAIResponses extracts normalized TokenUsage from an OpenAI Responses
// API usage block. Same semantics as Chat: InputTokens = total, CachedTokens
// is a subset.
func FromOpenAIResponses(u responses.ResponseUsage) *protocol.TokenUsage {
	cache := int(u.InputTokensDetails.CachedTokens)
	reasoning := int(u.OutputTokensDetails.ReasoningTokens)
	return protocol.NewTokenUsageFull(
		int(u.InputTokens)-cache,
		int(u.OutputTokens),
		cache,
		reasoning,
	)
}

// FromAnthropicMessage extracts normalized TokenUsage from an Anthropic v1
// (non-beta) Message usage block. CacheCreationInputTokens is added to
// InputTokens so the denominator covers all non-cache-read prompt cost.
func FromAnthropicMessage(u anthropic.Usage) *protocol.TokenUsage {
	return protocol.NewTokenUsageWithCache(
		int(u.InputTokens)+int(u.CacheCreationInputTokens),
		int(u.OutputTokens),
		int(u.CacheReadInputTokens),
	)
}

// FromAnthropicBetaMessage extracts normalized TokenUsage from an Anthropic
// beta BetaMessage usage block. Same normalization as the non-beta path.
func FromAnthropicBetaMessage(u anthropic.BetaUsage) *protocol.TokenUsage {
	return protocol.NewTokenUsageWithCache(
		int(u.InputTokens)+int(u.CacheCreationInputTokens),
		int(u.OutputTokens),
		int(u.CacheReadInputTokens),
	)
}

// ChatUsage converts normalized TokenUsage into an OpenAI Chat Completions
// CompletionUsage wire struct. OpenAI wire semantics: PromptTokens = TOTAL
// (uncached + cached), CachedTokens is a reported subset.
func ChatUsage(u *protocol.TokenUsage) openai.CompletionUsage {
	totalInput := u.InputTokens + u.CacheInputTokens
	cu := openai.CompletionUsage{
		PromptTokens:     int64(totalInput),
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      int64(totalInput + u.OutputTokens),
	}
	if u.CacheInputTokens > 0 {
		cu.PromptTokensDetails.CachedTokens = int64(u.CacheInputTokens)
	}
	if u.ReasoningTokens > 0 {
		cu.CompletionTokensDetails.ReasoningTokens = int64(u.ReasoningTokens)
	}
	return cu
}
