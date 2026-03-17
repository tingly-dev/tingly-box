//go:build e2e
// +build e2e

package smart_guide

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealAgentExecution tests the agent with real model calls.
// This test is skipped by default and only runs with -tags=e2e.
// Purpose: Verify e2e flow works without errors (model output is unpredictable).
// To run: go test -v -tags=e2e -run TestRealAgentExecution ./internal/remote_control/smart_guide/
func TestRealAgentExecution(t *testing.T) {
	// ============================================================================
	// CONFIGURATION
	// ============================================================================
	const (
		REAL_APIKey       = "tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjQwMzQwNSwiaWF0IjoxNzY2MzE3MDA1fQ.AHtmsHxGGJ0jtzvrTZMHC3kfl3Os94HOhMA-zXFtHXQ"
		REAL_BaseURL      = "http://localhost:12580/tingly/anthropic"
		REAL_Model        = "tingly-box"
		REAL_ProviderUUID = "bfd637ca-e9d6-11f0-b967-aaf5c138276e"
	)

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
	require.NoError(t, err, "Agent creation should succeed")
	require.NotNil(t, testAgent, "Agent should not be nil")

	t.Logf("✓ Agent created successfully with model: %s", REAL_Model)
	t.Logf("✓ Available tools: %d", len(testAgent.GetToolkit().GetSchemas()))

	// Test conversation
	ctx := context.Background()
	toolCtx := &ToolContext{
		ChatID:      "test-chat-real",
		ProjectPath: "/tmp",
	}

	testCases := []struct {
		name    string
		message string
	}{
		{
			name:    "Simple greeting",
			message: "Hello, can you help me?",
		},
		{
			name:    "Tool use - get status",
			message: "What's the current status?",
		},
		{
			name:    "Simple question",
			message: "What is the capital of France?",
		},
		{
			name:    "Tool use - ls command",
			message: "Please list the files in current directory with ls command",
		},
		{
			name:    "Multiple tools",
			message: "Show current directory with pwd, then list files with ls",
		},
		{
			name:    "Read file",
			message: "Read go.mod file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Sending: %s", tc.message)

			// Main assertions: no errors, non-nil response with content
			response, err := testAgent.ReplyWithContext(ctx, tc.message, toolCtx)

			// Check for errors - this is the key assertion for e2e
			assert.NoError(t, err, "Request should complete without errors")
			assert.NotNil(t, response, "Response should not be nil")

			if response != nil {
				// Check response has content
				content := response.Content
				assert.NotEmpty(t, content, "Response should have content")

				// Log the response for manual inspection (model output is unpredictable)
				responseText := response.GetTextContent()
				t.Logf("Response length: %d chars", len(responseText))

				// Print first 200 chars for visibility
				if len(responseText) > 200 {
					t.Logf("Response preview: %s...", responseText[:200])
				} else {
					t.Logf("Response: %s", responseText)
				}

				t.Logf("✓ Test case '%s' completed successfully", tc.name)
			}
		})
	}

	t.Logf("✓ All e2e tests passed - agent is working correctly")
}
