package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Example 1: Simple string prompt query
func exampleSimpleQuery() {
	ctx := context.Background()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "Say hello in one word",
		Options: &claude.QueryOptionsConfig{
			CWD:   "/tmp",
			Model: "claude-sonnet-4-6",
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 1: Simple Query ===")

	// Read messages using Next()
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		fmt.Printf("Message type: %s\n", msg.Type)

		// Handle different message types
		switch msg.Type {
		case "system":
			fmt.Printf("  Session: %s\n", msg.SessionID)
		case "assistant":
			if msg.Message != nil {
				if role, ok := msg.Message["role"].(string); ok {
					fmt.Printf("  Role: %s\n", role)
				}
			}
		case "result":
			fmt.Printf("  Result: %s\n", msg.Result)
		}
	}

	fmt.Println()
}

// Example 2: Using Messages channel
func exampleChannelQuery() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "What is 2+2? Answer with just the number.",
		Options: &claude.QueryOptionsConfig{
			CWD: "/tmp",
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 2: Channel Query ===")

	// Read from Messages channel
	for {
		select {
		case msg := <-query.Messages():
			fmt.Printf("Message: %s\n", msg.Type)
			if msg.Type == "result" {
				fmt.Printf("  Final result: %s\n", msg.Result)
			}
		case err := <-query.Errors():
			log.Printf("Error: %v", err)
			return
		case <-query.Done():
			fmt.Println("Query complete")
			return
		}
	}
}

// Example 3: Stream prompt with canCallTool
func exampleStreamPrompt() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build stream prompt
	builder := claude.NewStreamPromptBuilder()
	builder.AddUserMessage("List the files in current directory")

	launcher := claude.NewQueryLauncher(claude.Config{})

	// Define canCallTool callback
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		log.Printf("Tool permission requested: %s", toolName)
		// Auto-approve bash tool
		if toolName == "bash" {
			return map[string]interface{}{
				"approved": true,
			}, nil
		}
		// Deny other tools
		return nil, fmt.Errorf("tool %s not approved", toolName)
	}

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: builder.Messages(),
		Options: &claude.QueryOptionsConfig{
			CWD:         "/tmp",
			CanCallTool: canCallTool,
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 3: Stream Prompt with Tool Callback ===")

	// Process messages
	messageCount := 0
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}
		messageCount++

		if msg.Type == "tool_use" {
			fmt.Printf("Tool used: %s\n", msg.Request["name"])
		}
		if msg.Type == "result" {
			fmt.Printf("Final result received\n")
		}
	}
	fmt.Printf("Total messages: %d\n", messageCount)
}

// Example 4: Resume conversation
func exampleResume() {
	ctx := context.Background()

	launcher := claude.NewQueryLauncher(claude.Config{})

	// First, start a conversation and get session ID
	// (In real usage, you'd save this from a previous query)
	sessionID := "your-session-id-here"

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "Continue our conversation about Go",
		Options: &claude.QueryOptionsConfig{
			CWD:    "/tmp",
			Resume: sessionID,
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 4: Resume Conversation ===")

	for {
		msg, ok := query.Next()
		if !ok {
			break
		}
		fmt.Printf("Message: %s\n", msg.Type)
	}
}

// Example 5: With continue flag
func exampleContinue() {
	ctx := context.Background()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "What were we discussing?",
		Options: &claude.QueryOptionsConfig{
			CWD:                  "/tmp",
			ContinueConversation: true,
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 5: Continue Conversation ===")

	for {
		msg, ok := query.Next()
		if !ok {
			break
		}
		fmt.Printf("Message: %s\n", msg.Type)
	}
}

// Example 6: Using functional options
func exampleFunctionalOptions() {
	ctx := context.Background()

	query, err := claude.QueryWithContext(ctx, "Explain Go channels in one sentence",
		claude.WithModel("claude-sonnet-4-6"),
		claude.WithCWD("/tmp"),
		claude.WithAllowedTools("editor"),
	)
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 6: Functional Options ===")

	for {
		msg, ok := query.Next()
		if !ok {
			break
		}
		if msg.Type == "result" {
			fmt.Printf("Result: %s\n", msg.Result)
		}
	}
}

// Example 7: With interrupt support
func exampleInterrupt() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	launcher := claude.NewQueryLauncher(claude.Config{})

	query, err := launcher.Query(ctx, claude.QueryConfig{
		Prompt: "Count to 100 slowly",
		Options: &claude.QueryOptionsConfig{
			CWD: "/tmp",
		},
	})
	if err != nil {
		log.Printf("Query failed: %v", err)
		return
	}
	defer query.Close()

	fmt.Println("=== Example 7: Interrupt ===")

	// Read some messages then interrupt
	count := 0
	for {
		msg, ok := query.Next()
		if !ok {
			break
		}
		count++

		// After getting a few messages, send interrupt
		if count == 3 {
			fmt.Println("Sending interrupt...")
			query.Interrupt()
		}

		fmt.Printf("Message %d: %s\n", count, msg.Type)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run query_example.go <example>")
		fmt.Println("Examples:")
		fmt.Println("  1 - Simple query")
		fmt.Println("  2 - Channel query")
		fmt.Println("  3 - Stream prompt with tools")
		fmt.Println("  4 - Resume conversation")
		fmt.Println("  5 - Continue conversation")
		fmt.Println("  6 - Functional options")
		fmt.Println("  7 - Interrupt")
		os.Exit(1)
	}

	example := os.Args[1]

	switch example {
	case "1":
		exampleSimpleQuery()
	case "2":
		exampleChannelQuery()
	case "3":
		exampleStreamPrompt()
	case "4":
		exampleResume()
	case "5":
		exampleContinue()
	case "6":
		exampleFunctionalOptions()
	case "7":
		exampleInterrupt()
	default:
		fmt.Printf("Unknown example: %s\n", example)
		os.Exit(1)
	}
}
