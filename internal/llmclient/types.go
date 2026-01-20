package llmclient

// ProviderType represents the AI provider type
type ProviderType string

const (
	ProviderTypeOpenAI    ProviderType = "openai"
	ProviderTypeAnthropic ProviderType = "anthropic"
	ProviderTypeGoogle    ProviderType = "google"
)

// Client is the unified interface for AI provider clients
type Client interface {
	// ProviderType returns the type of provider this client implements
	ProviderType() ProviderType

	// Close closes any resources held by the client
	Close() error
}
