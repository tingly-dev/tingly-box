package smart_guide

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
)

// TestRealAgentExecution tests the agent with real model calls.
// This test is skipped by default and only runs when manually enabled.
// To run this test:
// 1. Fill in the REAL_* constants below with your actual credentials
// 2. Run: go test -v -run TestRealAgentExecution ./internal/remote_control/smart_guide/
func TestRealAgentExecution(t *testing.T) {
	//t.Skip("Real agent test - fill in REAL_* constants and remove Skip to run")

	// ============================================================================
	// CONFIGURATION - Fill these in with your real credentials
	// ============================================================================
	const (
		// REAL_APIKey is your API key for the model provider
		REAL_APIKey = "tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjQwMzQwNSwiaWF0IjoxNzY2MzE3MDA1fQ.AHtmsHxGGJ0jtzvrTZMHC3kfl3Os94HOhMA-zXFtHXQ"

		// REAL_BaseURL is the base URL for the model API (leave empty for official API)
		// MENTION: we only use anthropic since tingly-box can serve translation.
		REAL_BaseURL = "http://localhost:12580/tingly/anthropic"

		// REAL_Model is the model identifier (e.g., "claude-sonnet-4-6")
		REAL_Model = "tingly-box"

		// REAL_ProviderUUID is a fake UUID for testing (only used internally)
		REAL_ProviderUUID = "bfd637ca-e9d6-11f0-b967-aaf5c138276e"
	)

	// Validate configuration
	if REAL_APIKey == "sk-your-api-key-here" || REAL_APIKey == "" {
		t.Fatal("Please fill in REAL_APIKey with your actual API key")
	}

	// Create agent config
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          REAL_BaseURL,
		APIKey:           REAL_APIKey,
		Provider:         REAL_ProviderUUID,
		Model:            REAL_Model,
		GetStatusFunc: func(chatID string) (*StatusInfo, error) {
			return &StatusInfo{
				CurrentAgent:   "@tb",
				SessionID:      "test-session",
				ProjectPath:    "/tmp/test-project",
				WorkingDir:     "/tmp",
				HasRunningTask: false,
				Whitelisted:    true,
			}, nil
		},
		UpdateProjectFunc: func(chatID string, projectPath string) error {
			return nil
		},
	}

	// Create the agent
	testAgent, err := NewTinglyBoxAgent(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, testAgent)

	t.Logf("Agent created successfully with model: %s", REAL_Model)
	t.Logf("Agent Tools: %s", testAgent.GetToolkit().GetSchemas())

	// Test a simple conversation
	ctx := context.Background()
	toolCtx := &ToolContext{
		ChatID:      "test-chat-real",
		ProjectPath: "/tmp",
	}

	testCases := []struct {
		name     string
		message  string
		validate func(t *testing.T, response *message.Msg, err error)
	}{
		{
			name:    "Simple greeting",
			message: "Hello, can you help me?",
			validate: func(t *testing.T, response *message.Msg, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if response != nil {
					t.Logf("Response: %s", response.Content)
				}
			},
		},
		{
			name:    "Tool use - get status",
			message: "What's the current status?",
			validate: func(t *testing.T, response *message.Msg, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if response != nil {
					t.Logf("Response: %s", response.Content)
				}
			},
		},
		{
			name:    "Tool use - bash command",
			message: "Can you run 'pwd' to show the current directory?",
			validate: func(t *testing.T, response *message.Msg, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if response != nil {
					t.Logf("Response: %s", response.Content)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Sending message: %s", tc.message)
			response, err := testAgent.ReplyWithContext(ctx, tc.message, toolCtx)
			tc.validate(t, response, err)
		})
	}
}
