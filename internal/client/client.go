package client

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// NewOpenAIClient creates a new OpenAI client wrapper
// sessionID is used for session-scoped transport creation for OAuth providers
var NewOpenAIClient func(provider *typ.Provider, model string, sessionID typ.SessionID) (*OpenAIClient, error) = defaultNewOpenAIClient

// NewAnthropicClient creates a new Anthropic client wrapper
// sessionID is used for session-scoped transport creation for OAuth providers
var NewAnthropicClient func(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) = defaultNewAnthropicClient

// CloseClient closes a client if it implements Close()
func CloseClient(client protocol.Client) error {
	if client != nil {
		return client.Close()
	}
	return nil
}
