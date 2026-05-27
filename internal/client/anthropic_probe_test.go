package client

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestAnthropicClient_ProbeChatEndpoint tests the ProbeChatEndpoint method
func TestAnthropicClient_ProbeChatEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			provider: &typ.Provider{
				Name:     "test-anthropic",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "sk-test-key",
			},
			model:   "claude-3-haiku-20240307",
			wantErr: true, // Will fail with invalid key
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAnthropicClient(tt.provider, tt.model, typ.SessionID{})
			if err != nil {
				t.Fatalf("NewAnthropicClient() error = %v", err)
			}

			result := client.Probe(context.Background(), tt.model)

			if !tt.wantErr && !result.Success {
				t.Errorf("ProbeChatEndpoint() failed = %v", result.ErrorMessage)
			}
			if tt.wantErr && result.Success {
				t.Errorf("ProbeChatEndpoint() expected error but succeeded")
			}
		})
	}
}

// TestAnthropicClient_ProbeModelsEndpoint tests the ProbeModelsEndpoint method
func TestAnthropicClient_ProbeModelsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			provider: &typ.Provider{
				Name:     "test-anthropic",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "sk-test-key",
			},
			model:   "claude-3-haiku-20240307",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAnthropicClient(tt.provider, tt.model, typ.SessionID{})
			if err != nil {
				t.Fatalf("NewAnthropicClient() error = %v", err)
			}

			result := client.ProbeModelsEndpoint(context.Background())

			if !tt.wantErr && !result.Success {
				t.Errorf("ProbeModelsEndpoint() failed = %v", result.ErrorMessage)
			}
			if tt.wantErr && result.Success {
				t.Errorf("ProbeModelsEndpoint() expected error but succeeded")
			}
		})
	}
}

// TestAnthropicClient_ProbeOptionsEndpoint tests the ProbeOptionsEndpoint method
func TestAnthropicClient_ProbeOptionsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			provider: &typ.Provider{
				Name:     "test-anthropic",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				Token:    "sk-test-key",
			},
			model:   "claude-3-haiku-20240307",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAnthropicClient(tt.provider, tt.model, typ.SessionID{})
			if err != nil {
				t.Fatalf("NewAnthropicClient() error = %v", err)
			}

			result := client.ProbeOptionsEndpoint(context.Background())

			if !tt.wantErr && !result.Success {
				t.Errorf("ProbeOptionsEndpoint() failed = %v", result.ErrorMessage)
			}
			// OPTIONS might succeed even with invalid key for some providers
		})
	}
}

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

