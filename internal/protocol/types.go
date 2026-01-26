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
