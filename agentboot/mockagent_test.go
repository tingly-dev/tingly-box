package agentboot_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
)

// Example_mockAgent demonstrates using mock agent with agentboot
func Example_mockAgent() {
	// Create agentboot manager
	ab := agentboot.New(agentboot.Config{
		DefaultAgent:     agentboot.AgentTypeMockAgent,
		EnableStreamJSON: true,
	})

	// Create mock agent with custom config
	mockAgent := mock.NewAgent(mock.Config{
		MaxIterations: 3,
		StepDelay:     100 * time.Millisecond, // Fast for testing
		AutoApprove:   true,                   // Auto-approve for demo
	})

	// Register mock agent
	ab.RegisterAgent(agentboot.AgentTypeMockAgent, mockAgent)

	// Execute with context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agent, err := ab.GetDefaultAgent()
	if err != nil {
		fmt.Printf("Error getting agent: %v\n", err)
		return
	}
	result, err := agent.Execute(ctx, "Hello, mock agent!", agentboot.ExecutionOptions{
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

// Example_mockAgentWithPermission demonstrates mock agent with permission handler
func Example_mockAgentWithPermission() {
	// Create agentboot manager
	ab := agentboot.New(agentboot.Config{
		DefaultAgent: agentboot.AgentTypeMockAgent,
	})

	// Create permission handler with auto mode
	permHandler := permission.NewDefaultHandler(agentboot.PermissionConfig{
		DefaultMode: agentboot.PermissionModeAuto,
	})

	// Create mock agent
	mockAgent := mock.NewAgent(mock.Config{
		MaxIterations: 2,
		StepDelay:     50 * time.Millisecond,
	})
	mockAgent.SetPermissionHandler(permHandler)

	// Register and execute
	ab.RegisterAgent(agentboot.AgentTypeMockAgent, mockAgent)

	ctx := context.Background()
	agent, err := ab.GetDefaultAgent()
	if err != nil {
		fmt.Printf("Error getting agent: %v\n", err)
		return
	}
	result, err := agent.Execute(ctx, "Test with permission", agentboot.ExecutionOptions{})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Completed: %v\n", result.IsSuccess())
	// Output:
	// Completed: true
}
