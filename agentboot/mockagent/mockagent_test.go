package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Example_mockAgent demonstrates using mock agent with agentboot
func Example_mockAgent() {
	// Create mock agent with custom config
	mockAgent := NewAgent(Config{
		MaxIterations: 3,
		StepDelay:     100 * time.Millisecond, // Fast for testing
		AutoApprove:   true,                   // Auto-approve for demo
	})

	// Execute with context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mockAgent.Execute(ctx, "Hello, mock agent!", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Success: %v\n", result.IsSuccess())
	fmt.Printf("Steps: %d events\n", len(result.Events))
	// Output:
	// Success: true
	// Steps: 11 events
}

// Example_mockAgentWithHandler demonstrates mock agent with message handler
func Example_mockAgentWithHandler() {
	// Create mock agent
	mockAgent := NewAgent(Config{
		MaxIterations: 2,
		StepDelay:     50 * time.Millisecond,
		AutoApprove:   false, // Require manual approval
	})

	// Create message handler that auto-approves
	handler := agentboot.NewCompositeHandler().
		SetApprovalHandler(&autoApprovalHandler{}).
		SetAskHandler(&autoAskHandler{})

	// Execute with handler
	ctx := context.Background()
	result, err := mockAgent.Execute(ctx, "Test with handler", agentboot.ExecutionOptions{
		Handler:      handler,
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Completed: %v\n", result.IsSuccess())
	// Output:
	// Completed: true
}

// Example_mockAgentWithAskUserQuestion demonstrates mock agent with AskUserQuestion
func Example_mockAgentWithAskUserQuestion() {
	// Create mock agent that sends AskUserQuestion every 2 steps
	mockAgent := NewAgent(Config{
		MaxIterations:           4,
		StepDelay:               50 * time.Millisecond,
		AutoApprove:             false,
		AskUserQuestionFrequency: 2, // Every 2 steps, send AskUserQuestion
	})

	// Create handler that auto-approves everything
	handler := agentboot.NewCompositeHandler().
		SetApprovalHandler(&autoApprovalHandler{}).
		SetAskHandler(&autoAskHandler{})

	// Execute
	ctx := context.Background()
	result, err := mockAgent.Execute(ctx, "Test with AskUserQuestion", agentboot.ExecutionOptions{
		Handler:      handler,
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Completed: %v\n", result.IsSuccess())
	fmt.Printf("Events: %d\n", len(result.Events))
	// Output:
	// Completed: true
	// Events: 14
}

// autoApprovalHandler is a simple handler that auto-approves all permissions
type autoApprovalHandler struct{}

func (h *autoApprovalHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return agentboot.PermissionResult{Approved: true, UpdatedInput: req.Input}, nil
}

// autoAskHandler is a simple handler that auto-approves all asks
type autoAskHandler struct{}

func (h *autoAskHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	// For AskUserQuestion, add answers to the input
	if req.ToolName == "AskUserQuestion" && req.Input != nil {
		questions, ok := req.Input["questions"].([]interface{})
		if ok && len(questions) > 0 {
			// Provide default answers (select first option for each question)
			answers := make(map[string]interface{})
			for i := range questions {
				answers[fmt.Sprintf("%d", i)] = "Option A"
			}
			updatedInput := make(map[string]interface{})
			for k, v := range req.Input {
				updatedInput[k] = v
			}
			updatedInput["answers"] = answers
			return agentboot.AskResult{
				ID:           req.ID,
				Approved:     true,
				UpdatedInput: updatedInput,
			}, nil
		}
	}
	return agentboot.AskResult{ID: req.ID, Approved: true}, nil
}