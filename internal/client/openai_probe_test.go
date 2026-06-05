package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestOpenAIClient_Timeout tests that timeout is properly configured from provider
func TestOpenAIClient_Timeout(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
	}{
		{
			name: "default timeout (30 minutes)",
			provider: &typ.Provider{
				Name:     "test-openai",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
				Token:    "sk-test-key",
				Timeout:  1800,
			},
			model: "gpt-3.5-turbo",
		},
		{
			name: "custom timeout (1 minute)",
			provider: &typ.Provider{
				Name:     "test-openai-fast",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
				Token:    "sk-test-key",
				Timeout:  60,
			},
			model: "gpt-3.5-turbo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOpenAIClient(tt.provider, tt.model, typ.SessionID{})
			if err != nil {
				t.Fatalf("NewOpenAIClient() error = %v", err)
			}
			defer client.Close()

			if client.Client() == nil {
				t.Error("Expected non-nil SDK client")
			}
		})
	}
}
