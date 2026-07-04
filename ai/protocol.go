// Package protocol provides types for AI protocol conversion and client interfaces.
// This is the public API for protocol-related types.
package ai

import (
	"github.com/openai/openai-go/v3/shared"
)

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
	APIStyleGoogle    APIStyle = "google"
)

// APIType represents the target API style for protocol conversion
type APIType string

const (
	// TypeOpenAIChat converts requests to OpenAI Chat Completions format
	TypeOpenAIChat APIType = "openai_chat"

	// TypeOpenAIResponses converts requests to OpenAI Responses API format
	TypeOpenAIResponses APIType = "openai_responses"

	// TypeAnthropicV1 converts requests to Anthropic v1 Messages API format
	TypeAnthropicV1 APIType = "anthropic_v1"

	// TypeAnthropicBeta converts requests to Anthropic v1beta Messages API format
	TypeAnthropicBeta APIType = "anthropic_beta"

	// TypeGoogle converts requests to Google Gemini API format
	TypeGoogle APIType = "google"
)

// Client is the unified interface for AI provider clients
type Client interface {
	// APIStyle returns the type of provider this client implements
	APIStyle() APIStyle

	// Close closes any resources held by the client
	Close() error
}

// OpenAIConfig contains additional metadata that may be used by provider transforms
type OpenAIConfig struct {
	// HasThinking indicates whether the request contains thinking content
	// This can be used by providers like DeepSeek to handle reasoning_content
	HasThinking bool

	// ReasoningEffort specifies the reasoning effort level for OpenAI-compatible APIs
	// Valid values: "none", "minimal", "low", "medium", "high", "xhigh"
	// Defaults to "low" when HasThinking is true
	ReasoningEffort shared.ReasoningEffort

	// Future fields can be added here as needed for provider-specific transformations
}

// TokenUsage represents comprehensive token usage statistics.
// This structure provides a unified interface for tracking token usage
// across all supported protocols (OpenAI, Anthropic, Google).
type TokenUsage struct {
	// InputTokens is the number of input/prompt tokens consumed (excluding cache)
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of output/completion tokens consumed
	OutputTokens int `json:"output_tokens"`

	// CacheInputTokens is the number of cache-read-hit tokens consumed.
	// Cache-write cost is tracked separately in CacheWriteTokens.
	// Note: Anthropic normalization folds CacheWriteTokens into InputTokens
	// (InputTokens += cache_creation), so InputTokens already covers write cost.
	// Total prompt cost = InputTokens + CacheInputTokens.
	CacheInputTokens int `json:"cache_input_tokens,omitempty"`

	// CacheReadTokens and CacheWriteTokens provide detail for billing.
	// CacheReadTokens  = cache-read hits (same as CacheInputTokens).
	// CacheWriteTokens = cache writes; folded into InputTokens in Anthropic
	// normalization; zero for OpenAI (no wire-level write concept).
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`

	// ReasoningTokens is the number of tokens used for internal reasoning
	// (e.g. o1/o3 reasoning models). These are a subset of OutputTokens.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`

	// SystemTokens represents tokens consumed by system-level operations
	// such as prompt templates, system instructions, or framework overhead
	SystemTokens int `json:"system_tokens,omitempty"`
}

// TotalTokens returns the total tokens consumed (input + output, excluding cache).
// Cache tokens are tracked separately for cost calculation purposes.
func (u *TokenUsage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// HasUsage returns true if any token count is non-zero.
func (u *TokenUsage) HasUsage() bool {
	return u.InputTokens > 0 || u.OutputTokens > 0 ||
		u.CacheInputTokens > 0 || u.SystemTokens > 0
}

// HasCacheUsage returns true if cache tokens are present.
func (u *TokenUsage) HasCacheUsage() bool {
	return u.CacheInputTokens > 0
}

// ToAnthropicUsageMap converts canonical usage into Anthropic message usage shape.
func (u *TokenUsage) ToAnthropicUsageMap() map[string]interface{} {
	usage := map[string]interface{}{
		"input_tokens":  u.InputTokens,
		"output_tokens": u.OutputTokens,
	}
	if u.CacheReadTokens > 0 {
		usage["cache_read_input_tokens"] = u.CacheReadTokens
	}
	if u.CacheWriteTokens > 0 {
		usage["cache_creation_input_tokens"] = u.CacheWriteTokens
	}
	return usage
}

// ToAnthropicMessageDeltaUsageMap converts canonical usage into Anthropic stream
// message_delta usage shape for protocol conversion. Native Anthropic streams put
// input usage on message_start, but converters such as OpenAI→Anthropic only get
// authoritative input usage at the terminal upstream event, so the final
// message_delta carries the complete normalized usage.
func (u *TokenUsage) ToAnthropicMessageDeltaUsageMap() map[string]interface{} {
	return u.ToAnthropicUsageMap()
}

// ToOpenAIChatUsageMap converts canonical usage into OpenAI Chat Completions
// usage shape. Prompt/input tokens on the wire include cached tokens.
func (u *TokenUsage) ToOpenAIChatUsageMap() map[string]interface{} {
	inputTokens := u.InputTokens + u.CacheInputTokens
	usage := map[string]interface{}{
		"prompt_tokens":     inputTokens,
		"completion_tokens": u.OutputTokens,
		"total_tokens":      inputTokens + u.OutputTokens,
	}
	if u.CacheInputTokens > 0 {
		usage["prompt_tokens_details"] = map[string]interface{}{
			"cached_tokens": u.CacheInputTokens,
		}
	}
	if u.ReasoningTokens > 0 {
		usage["completion_tokens_details"] = map[string]interface{}{
			"reasoning_tokens": u.ReasoningTokens,
		}
	}
	return usage
}

// ToOpenAIResponsesUsageMap converts canonical usage into OpenAI Responses API
// usage shape. Input tokens on the wire include cached tokens.
func (u *TokenUsage) ToOpenAIResponsesUsageMap() map[string]interface{} {
	inputTokens := u.InputTokens + u.CacheInputTokens
	usage := map[string]interface{}{
		"input_tokens":  inputTokens,
		"output_tokens": u.OutputTokens,
		"total_tokens":  inputTokens + u.OutputTokens,
	}
	if u.CacheInputTokens > 0 {
		usage["input_tokens_details"] = map[string]interface{}{
			"cached_tokens": u.CacheInputTokens,
		}
	}
	if u.ReasoningTokens > 0 {
		usage["output_tokens_details"] = map[string]interface{}{
			"reasoning_tokens": u.ReasoningTokens,
		}
	}
	return usage
}

// NewTokenUsage creates a new TokenUsage with the given token counts.
func NewTokenUsage(inputTokens, outputTokens int) *TokenUsage {
	return &TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

// NewTokenUsageWithCache creates a new TokenUsage with cache token count.
func NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens int) *TokenUsage {
	return NewTokenUsageWithCacheDetails(inputTokens, outputTokens, cacheTokens, 0)
}

// NewTokenUsageWithCacheDetails creates a TokenUsage with cache read/write detail counts.
func NewTokenUsageWithCacheDetails(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) *TokenUsage {
	return &TokenUsage{
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		CacheInputTokens: cacheReadTokens,
		CacheReadTokens:  cacheReadTokens,
		CacheWriteTokens: cacheWriteTokens,
	}
}

// NewTokenUsageFull creates a TokenUsage with cache and reasoning token counts.
func NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens int) *TokenUsage {
	usage := NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
	usage.ReasoningTokens = reasoningTokens
	return usage
}

// ZeroTokenUsage returns a TokenUsage with zero values.
func ZeroTokenUsage() *TokenUsage {
	return &TokenUsage{}
}
