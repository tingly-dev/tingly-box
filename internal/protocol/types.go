package protocol

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

// UsageStat represents token usage statistics returned by stream handlers.
// This is used to propagate usage information from protocol conversion
// handlers back to the server layer for tracking.
type UsageStat struct {
	// InputTokens is the number of input/prompt tokens consumed
	InputTokens int

	// OutputTokens is the number of output/completion tokens consumed
	OutputTokens int
}

// TotalTokens returns the sum of input and output tokens.
func (u *UsageStat) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// HasUsage returns true if either input or output tokens are non-zero.
func (u *UsageStat) HasUsage() bool {
	return u.InputTokens > 0 || u.OutputTokens > 0
}

// NewUsageStat creates a new UsageStat with the given token counts.
func NewUsageStat(inputTokens, outputTokens int) UsageStat {
	return UsageStat{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

// ZeroUsageStat returns a UsageStat with zero values.
func ZeroUsageStat() UsageStat {
	return UsageStat{}
}
