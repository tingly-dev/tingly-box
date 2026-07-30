// Package usage centralizes token extraction and normalization logic for all
// supported provider protocols. Every handler calls into this package instead
// of re-implementing provider-specific rules inline.
//
// Normalization rules:
//   - OpenAI (Chat / Responses): prompt_tokens = total (cached + written +
//     uncached). Store inputTokens = total - cached so the frontend ratio
//     formula gives cache_read / (cache_read + uncached) = correct hit rate.
//     cache_write_tokens (gpt-5.6+) stays inside inputTokens because it is
//     billed at a premium rate, and is also reported as CacheWriteTokens.
//   - Anthropic: input_tokens = uncached only; cache_creation_input_tokens is
//     an additional write cost that belongs in the denominator.
//     Store inputTokens = input + creation so the formula covers total prompt cost.
//
// Both sides therefore agree: inputTokens = uncached + written, and
// CacheWriteTokens is a subset of inputTokens, never an addition to it.
package usage

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// FromOpenAIChatCompletion extracts normalized TokenUsage from an OpenAI Chat
// Completions usage block. CachedTokens and CacheWriteTokens are disjoint
// SUBSETS of PromptTokens. Only the read hits are subtracted: writes are billed
// at 1.25x the uncached input rate (gpt-5.6+), so they stay inside InputTokens
// and are reported separately for cost attribution — mirroring how Anthropic's
// cache_creation_input_tokens is folded in.
func FromOpenAIChatCompletion(u openai.CompletionUsage) *protocol.TokenUsage {
	cacheRead := int(u.PromptTokensDetails.CachedTokens)
	cacheWrite := int(u.PromptTokensDetails.CacheWriteTokens)
	reasoning := int(u.CompletionTokensDetails.ReasoningTokens)
	return protocol.NewTokenUsageFull(
		int(u.PromptTokens)-cacheRead,
		int(u.CompletionTokens),
		cacheRead,
		cacheWrite,
		reasoning,
	)
}

// FromOpenAIResponses extracts normalized TokenUsage from an OpenAI Responses
// API usage block. Same semantics as Chat: InputTokens = total, CachedTokens
// and CacheWriteTokens are subsets.
func FromOpenAIResponses(u responses.ResponseUsage) *protocol.TokenUsage {
	cacheRead := int(u.InputTokensDetails.CachedTokens)
	cacheWrite := int(u.InputTokensDetails.CacheWriteTokens)
	reasoning := int(u.OutputTokensDetails.ReasoningTokens)
	return protocol.NewTokenUsageFull(
		int(u.InputTokens)-cacheRead,
		int(u.OutputTokens),
		cacheRead,
		cacheWrite,
		reasoning,
	)
}

// FromAnthropicMessage extracts normalized TokenUsage from an Anthropic v1
// (non-beta) Message usage block. CacheCreationInputTokens is added to
// InputTokens so the denominator covers all non-cache-read prompt cost.
func FromAnthropicMessage(u anthropic.Usage) *protocol.TokenUsage {
	return protocol.NewTokenUsageWithCacheDetails(
		int(u.InputTokens)+int(u.CacheCreationInputTokens),
		int(u.OutputTokens),
		int(u.CacheReadInputTokens),
		int(u.CacheCreationInputTokens),
	)
}

// FromAnthropicBetaMessage extracts normalized TokenUsage from an Anthropic
// beta BetaMessage usage block. Same normalization as the non-beta path.
func FromAnthropicBetaMessage(u anthropic.BetaUsage) *protocol.TokenUsage {
	return protocol.NewTokenUsageWithCacheDetails(
		int(u.InputTokens)+int(u.CacheCreationInputTokens),
		int(u.OutputTokens),
		int(u.CacheReadInputTokens),
		int(u.CacheCreationInputTokens),
	)
}

// ChatUsage converts normalized TokenUsage into an OpenAI Chat Completions
// CompletionUsage wire struct. OpenAI wire semantics: PromptTokens = TOTAL
// (uncached + cached), CachedTokens is a reported subset.
func ChatUsage(u *protocol.TokenUsage) openai.CompletionUsage {
	totalInput := u.InputTokens + u.CacheReadTokens
	cu := openai.CompletionUsage{
		PromptTokens:     int64(totalInput),
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      int64(totalInput + u.OutputTokens),
	}
	if u.CacheReadTokens > 0 {
		cu.PromptTokensDetails.CachedTokens = int64(u.CacheReadTokens)
	}
	if u.CacheWriteTokens > 0 {
		cu.PromptTokensDetails.CacheWriteTokens = int64(u.CacheWriteTokens)
	}
	if u.ReasoningTokens > 0 {
		cu.CompletionTokensDetails.ReasoningTokens = int64(u.ReasoningTokens)
	}
	return cu
}
