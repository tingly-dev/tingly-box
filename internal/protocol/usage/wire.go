package usage

import (
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// TokenUsage → wire output constructors.
//
// The canonical *protocol.TokenUsage is the single source of truth for token
// accounting; these functions are the ONLY place that turns it into a wire
// usage shape. Every handler/stream converter calls them instead of rebuilding
// the prompt-total + cache-detail + reasoning-detail logic inline.
//
// Shared semantics (mirrors protocol.TokenUsage docs):
//   - prompt/input tokens on the wire = TOTAL = uncached + cache_read +
//     cache_write (InputTokens already folds writes in, so only read hits are
//     added back via PromptTotalTokens).
//   - cached_tokens and cache_write_tokens are reported as disjoint subsets.
//   - cache_write_tokens / details blocks are omitted when there is nothing to
//     report — an absent value means "channel does not report it", which is
//     distinct from an explicit zero.
//
// (Exception: ResponsesInputTokensDetailsWire keeps cached_tokens/reasoning
// always-present per its own doc — see that struct.)

// ToChatStreamUsageWire converts normalized TokenUsage into the Chat Completions
// stream usage wire shape.
func ToChatStreamUsageWire(u *protocol.TokenUsage) *wire.ChatStreamUsage {
	if u == nil {
		return nil
	}
	totalInput := int64(u.PromptTotalTokens())
	built := &wire.ChatStreamUsage{
		PromptTokens:     totalInput,
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      totalInput + int64(u.OutputTokens),
	}
	if u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
		built.PromptTokensDetails = &wire.ChatStreamPromptTokenDetails{
			CachedTokens:     int64(u.CacheReadTokens),
			CacheWriteTokens: int64(u.CacheWriteTokens),
		}
	}
	if u.ReasoningTokens > 0 {
		built.CompletionTokensDetails = &wire.ChatStreamOutputTokenDetails{ReasoningTokens: int64(u.ReasoningTokens)}
	}
	return built
}

// ToChatUsageWire converts normalized TokenUsage into the non-streaming Chat
// Completions usage wire shape.
func ToChatUsageWire(u *protocol.TokenUsage) wire.ChatCompletionUsageWire {
	if u == nil {
		return wire.ChatCompletionUsageWire{}
	}
	totalInput := int64(u.PromptTotalTokens())
	built := wire.ChatCompletionUsageWire{
		PromptTokens:     totalInput,
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      totalInput + int64(u.OutputTokens),
	}
	if u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
		built.PromptTokensDetails = &wire.ChatCompletionPromptDetailsWire{
			CachedTokens:     int64(u.CacheReadTokens),
			CacheWriteTokens: int64(u.CacheWriteTokens),
		}
	}
	if u.ReasoningTokens > 0 {
		built.CompletionTokensDetails = &wire.ChatCompletionOutputDetailsWire{
			ReasoningTokens: int64(u.ReasoningTokens),
		}
	}
	return built
}

// ToResponsesUsageWire converts normalized TokenUsage into the Responses API
// usage wire shape.
func ToResponsesUsageWire(u *protocol.TokenUsage) *wire.ResponsesUsageWire {
	if u == nil {
		return nil
	}
	inputTokens := int64(u.PromptTotalTokens())
	built := &wire.ResponsesUsageWire{
		InputTokens:  inputTokens,
		OutputTokens: int64(u.OutputTokens),
		TotalTokens:  inputTokens + int64(u.OutputTokens),
		InputTokensDetails: wire.ResponsesInputTokensDetailsWire{
			CachedTokens:     int64(u.CacheReadTokens),
			CacheWriteTokens: int64(u.CacheWriteTokens),
		},
		OutputTokensDetails: wire.ResponsesOutputTokensDetailsWire{
			ReasoningTokens: int64(u.ReasoningTokens),
		},
	}
	// Emit the retention-tier split only when the channel reported one — an
	// absent details block means "tier unknown", matching the omitempty on
	// the wire struct.
	if u.CacheWriteTTL30m > 0 || u.CacheWriteTTL12h > 0 {
		built.InputTokensDetails.CacheWriteTokensDetails = &wire.ResponsesCacheWriteTokensDetailsWire{
			Ttl30mTokens: int64(u.CacheWriteTTL30m),
			Ttl12hTokens: int64(u.CacheWriteTTL12h),
		}
	}
	return built
}
