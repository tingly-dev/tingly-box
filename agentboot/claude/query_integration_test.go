package claude_test

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// TestQuerySimpleSimpleIntegration is an integration test that runs actual Claude CLI
// Skip this test if Claude CLI is not available
func TestQuerySimpleIntegration(t *testing.T) {
	t.Skip("Integration test - requires Claude CLI")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "Say hello in one word",
		Options: &claude.QueryOptionsConfig{
			CWD:   "/tmp",
			Model: "claude-sonnet-4-6",
		},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	// Read at least some messages
	messageCount := 0
	timeout := time.After(90 * time.Second)

loop:
	for {
		select {
		case msg := <-query.Messages():
			messageCount++
			t.Logf("Received message: %s", msg.Type)

			// We should get at least system, assistant, and result
			if messageCount >= 3 {
				break loop
			}

		case err := <-query.Errors():
			t.Fatalf("Query error: %v", err)

		case <-timeout:
			t.Fatalf("Timeout waiting for messages, got %d", messageCount)

		case <-query.Done():
			t.Log("Query completed")
			break loop
		}
	}

	if messageCount < 3 {
		t.Errorf("Expected at least 3 messages, got %d", messageCount)
	}
}

// TestQueryWithCanCallToolIntegration tests tool permission handling
func TestQueryWithCanCallToolIntegration(t *testing.T) {
	t.Skip("Integration test - requires Claude CLI")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Build stream prompt
	builder := claude.NewStreamPromptBuilder()
	builder.AddUserMessage("List the files in /tmp using bash")

	// Track tool calls
	toolCalls := make([]string, 0)

	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		t.Logf("Tool permission requested: %s", toolName)
		toolCalls = append(toolCalls, toolName)

		// Auto-approve bash tool
		if toolName == "bash" {
			return map[string]interface{}{
				"approved": true,
			}, nil
		}
		return nil, nil
	}

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: builder.Messages(),
		Options: &claude.QueryOptionsConfig{
			CWD:         "/tmp",
			CanCallTool: canCallTool,
		},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer query.Close()

	// Read messages
	timeout := time.After(120 * time.Second)
	for {
		select {
		case msg := <-query.Messages():
			t.Logf("Message: %s", msg.Type)
			if msg.Type == "result" {
				return // Success
			}

		case err := <-query.Errors():
			t.Logf("Error (may be expected): %v", err)

		case <-timeout:
			t.Fatal("Timeout")

		case <-query.Done():
			t.Log("Query completed")
			return
		}
	}
}

// TestQueryWithOptionsIntegration tests various query options
func TestQueryWithOptionsIntegration(t *testing.T) {
	t.Skip("Integration test - requires Claude CLI")

	tests := []struct {
		name    string
		prompt  string
		options []claude.QueryOption
	}{
		{
			name:   "With model",
			prompt: "What is 1+1?",
			options: []claude.QueryOption{
				claude.WithModel("claude-sonnet-4-6"),
			},
		},
		{
			name:   "With allowed tools",
			prompt: "Say hello",
			options: []claude.QueryOption{
				claude.WithAllowedTools("editor"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			query, err := claude.QueryWithContext(ctx, tt.prompt, tt.options...)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			defer query.Close()

			// Wait for completion
			timeout := time.After(90 * time.Second)
			for {
				select {
				case <-query.Messages():
					// Consume messages
				case <-query.Done():
					return
				case <-timeout:
					t.Fatal("Timeout")
				}
			}
		})
	}
}
