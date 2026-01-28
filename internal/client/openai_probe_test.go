package client

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestOpenAIClient_ProbeChatEndpoint tests the ProbeChatEndpoint method
func TestOpenAIClient_ProbeChatEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		model    string
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			// This test would require a real API key to run
			// In a real scenario, you might use environment variables or test fixtures
			provider: &typ.Provider{
				Name:     "test-openai",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
				Token:    "sk-test-key",
			},
			model:   "gpt-3.5-turbo",
			wantErr: true, // Will fail with invalid key
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOpenAIClient(tt.provider)
			if err != nil {
				t.Fatalf("NewOpenAIClient() error = %v", err)
			}

			result := client.ProbeChatEndpoint(context.Background(), tt.model)

			if !tt.wantErr && !result.Success {
				t.Errorf("ProbeChatEndpoint() failed = %v", result.ErrorMessage)
			}
			if tt.wantErr && result.Success {
				t.Errorf("ProbeChatEndpoint() expected error but succeeded")
			}
		})
	}
}

// TestOpenAIClient_ProbeModelsEndpoint tests the ProbeModelsEndpoint method
func TestOpenAIClient_ProbeModelsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			provider: &typ.Provider{
				Name:     "test-openai",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
				Token:    "sk-test-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOpenAIClient(tt.provider)
			if err != nil {
				t.Fatalf("NewOpenAIClient() error = %v", err)
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

// TestOpenAIClient_ProbeOptionsEndpoint tests the ProbeOptionsEndpoint method
func TestOpenAIClient_ProbeOptionsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		wantErr  bool
	}{
		{
			name: "skip live test - requires valid API key",
			provider: &typ.Provider{
				Name:     "test-openai",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
				Token:    "sk-test-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOpenAIClient(tt.provider)
			if err != nil {
				t.Fatalf("NewOpenAIClient() error = %v", err)
			}

			result := client.ProbeOptionsEndpoint(context.Background())

			if !tt.wantErr && !result.Success {
				t.Errorf("ProbeOptionsEndpoint() failed = %v", result.ErrorMessage)
			}
			// OPTIONS might succeed even with invalid key for some providers
		})
	}
}
