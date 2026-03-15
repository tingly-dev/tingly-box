package protocol

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/shared"
)

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
	APIStyleGoogle    APIStyle = "google"
)

// ChatGPTBackendAPIBase is the API base URL for ChatGPT/Codex OAuth provider
const ChatGPTBackendAPIBase = "https://chatgpt.com/backend-api"

// Client is the unified interface for AI provider clients
type Client interface {
	// APIStyle returns the type of provider this client implements
	APIStyle() APIStyle

	// Close closes any resources held by the client
	Close() error
}

// Transformer defines the interface for request compacting transformations.
// Each handler method is responsible for a different request model type.
type Transformer interface {
	// HandleV1 handles compacting for Anthropic v1 requests.
	HandleV1(req *anthropic.MessageNewParams) error

	// HandleV1Beta handles compacting for Anthropic v1beta requests.
	HandleV1Beta(req *anthropic.BetaMessageNewParams) error
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

	// CacheInputTokens is the number of cache-related tokens consumed
	// (includes both cache creation and cache read operations)
	CacheInputTokens int `json:"cache_input_tokens,omitempty"`

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

// NewTokenUsage creates a new TokenUsage with the given token counts.
func NewTokenUsage(inputTokens, outputTokens int) *TokenUsage {
	return &TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

// NewTokenUsageWithCache creates a new TokenUsage with cache token count.
func NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens int) *TokenUsage {
	return &TokenUsage{
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		CacheInputTokens: cacheTokens,
	}
}

// ZeroTokenUsage returns a TokenUsage with zero values.
func ZeroTokenUsage() *TokenUsage {
	return &TokenUsage{}
}
