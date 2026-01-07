package llmclient

import (
	"tingly-box/internal/typ"
)

// NewOpenAIClient creates a new OpenAI client wrapper
var NewOpenAIClient func(provider *typ.Provider) (*OpenAIClient, error) = defaultNewOpenAIClient

// NewAnthropicClient creates a new Anthropic client wrapper
var NewAnthropicClient func(provider *typ.Provider) (*AnthropicClient, error) = defaultNewAnthropicClient

// CloseClient closes a client if it implements Close()
func CloseClient(client Client) error {
	if client != nil {
		return client.Close()
	}
	return nil
}
