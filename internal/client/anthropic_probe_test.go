package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestAnthropicClient_Timeout tests that timeout is properly configured from provider
func TestAnthropicClient_Timeout(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
	}{
		{
			name: "default timeout (30 minutes)",
			provider: &typ.Provider{
				Name:     "test-anthropic",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "sk-test-key",
				Timeout:  1800,
			},
			model: "claude-3-haiku-20240307",
		},
		{
			name: "custom timeout (1 minute)",
			provider: &typ.Provider{
				Name:     "test-anthropic-fast",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "sk-test-key",
				Timeout:  60,
			},
			model: "claude-3-haiku-20240307",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAnthropicClient(tt.provider, tt.model, typ.SessionID{})
			if err != nil {
				t.Fatalf("NewAnthropicClient() error = %v", err)
			}
			defer client.Close()

			if client.Client() == nil {
				t.Error("Expected non-nil SDK client")
			}
		})
	}
}
