package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Color codes for output
const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
)

// StdinHandler implements MessageHandler for stdin/stdout interaction
type StdinHandler struct {
	Debug bool
}

// NewStdinHandler creates a new StdinHandler
func NewStdinHandler() *StdinHandler {
	return &StdinHandler{}
}

// OnApproval implements agentboot.ApprovalHandler
func (h *StdinHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// AskUserQuestion is a special case - present options for user to select
	if req.ToolName == "AskUserQuestion" {
		return h.handleAskUserQuestionApproval(ctx, req)
	}

	// Regular tool permission - show y/n prompt
	fmt.Printf("\r[Tool Permission] Claude wants to use: %s\n", req.ToolName)

	// Show relevant input details
	if cmd, ok := req.Input["command"].(string); ok {
		fmt.Printf("Command: %s\n", cmd)
	} else if h.Debug {
		fmt.Printf("Input: %+v\n", req.Input)
	}

	fmt.Printf("Allow? (y=yes/n=no/a=always): ")

	// Read user input
	type result struct {
		response string
		err      error
	}
	resultChan := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		resultChan <- result{response: response, err: err}
	}()

	// Wait for input or context cancellation
	select {
	case <-ctx.Done():
		return agentboot.PermissionResult{Approved: false}, ctx.Err()
	case r := <-resultChan:
		if r.err != nil {
			return agentboot.PermissionResult{Approved: false}, r.err
		}

		switch r.response {
		case "y", "yes":
			return agentboot.PermissionResult{
				Approved:     true,
				UpdatedInput: req.Input,
			}, nil
		case "a", "always":
			return agentboot.PermissionResult{
				Approved:     true,
				UpdatedInput: req.Input,
				Remember:     true,
			}, nil
		case "n", "no":
			return agentboot.PermissionResult{Approved: false}, nil
		default:
			return agentboot.PermissionResult{Approved: false}, nil
		}
	}
}

// handleAskUserQuestionApproval handles AskUserQuestion with option selection
func (h *StdinHandler) handleAskUserQuestionApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	fmt.Printf("\r%s[Question]%s\n", ColorYellow, ColorReset)

	answers := make(map[string]interface{})

	for i, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		questionText, _ := question["question"].(string)
		header, _ := question["header"].(string)

		if header != "" {
			fmt.Printf("\n%s[%s]%s\n", ColorCyan, header, ColorReset)
		}
		fmt.Printf("%s\n", questionText)

		options, ok := question["options"].([]interface{})
		if !ok || len(options) == 0 {
			continue
		}

		fmt.Printf("\nOptions:\n")
		for j, opt := range options {
			option, ok := opt.(map[string]interface{})
			if !ok {
				continue
			}
			label, _ := option["label"].(string)
			desc, _ := option["description"].(string)
			if desc != "" {
				fmt.Printf("  %s%d%s. %s - %s\n", ColorGreen, j+1, ColorReset, label, desc)
			} else {
				fmt.Printf("  %s%d%s. %s\n", ColorGreen, j+1, ColorReset, label)
			}
		}

		fmt.Printf("\n%sSelect option (1-%d) or type label: %s", ColorGreen, len(options), ColorReset)

		type result struct {
			response string
			err      error
		}
		resultChan := make(chan result, 1)

		go func() {
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			response = strings.TrimSpace(response)
			resultChan <- result{response: response, err: err}
		}()

		select {
		case <-ctx.Done():
			return agentboot.PermissionResult{Approved: false}, ctx.Err()
		case r := <-resultChan:
			if r.err != nil {
				return agentboot.PermissionResult{Approved: false}, r.err
			}

			var selectedIndex int = -1
			var selectedLabel string

			var num int
			if _, err := fmt.Sscanf(r.response, "%d", &num); err == nil {
				if num >= 1 && num <= len(options) {
					selectedIndex = num - 1
				}
			}

			if selectedIndex < 0 {
				for j, opt := range options {
					if option, ok := opt.(map[string]interface{}); ok {
						if label, ok := option["label"].(string); ok {
							if strings.EqualFold(label, r.response) {
								selectedIndex = j
								break
							}
						}
					}
				}
			}

			if selectedIndex < 0 {
				selectedLabel = r.response
			} else if selectedIndex >= 0 && selectedIndex < len(options) {
				if option, ok := options[selectedIndex].(map[string]interface{}); ok {
					if label, ok := option["label"].(string); ok {
						selectedLabel = label
					}
				}
			}

			answers[fmt.Sprintf("%d", i)] = selectedLabel
		}
	}

	updatedInput := make(map[string]interface{})
	for k, v := range req.Input {
		updatedInput[k] = v
	}
	updatedInput["answers"] = answers

	return agentboot.PermissionResult{
		Approved:     true,
		UpdatedInput: updatedInput,
	}, nil
}

// OnAsk implements agentboot.AskHandler
func (h *StdinHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	if req.ToolName == "AskUserQuestion" {
		return h.handleAskUserQuestion(ctx, req)
	}

	if req.Type == "text_input" || req.Message != "" {
		return h.handleTextInput(ctx, req)
	}

	return agentboot.AskResult{ID: req.ID, Approved: true}, nil
}

func (h *StdinHandler) handleTextInput(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	if req.Message != "" {
		fmt.Printf("\r%s[Input Required]%s %s\n", ColorYellow, ColorReset, req.Message)
	} else {
		fmt.Printf("\r%s[Input Required]%s\n", ColorYellow, ColorReset)
	}
	fmt.Printf("%sEnter response%s: ", ColorGreen, ColorReset)

	type result struct {
		response string
		err      error
	}
	resultChan := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		response = strings.TrimSpace(response)
		resultChan <- result{response: response, err: err}
	}()

	select {
	case <-ctx.Done():
		return agentboot.AskResult{ID: req.ID, Approved: false}, ctx.Err()
	case r := <-resultChan:
		if r.err != nil {
			return agentboot.AskResult{ID: req.ID, Approved: false}, r.err
		}
		return agentboot.AskResult{
			ID:       req.ID,
			Approved: true,
			Response: r.response,
		}, nil
	}
}

func (h *StdinHandler) handleAskUserQuestion(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return agentboot.AskResult{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	fmt.Printf("\r%s[Question]%s\n", ColorYellow, ColorReset)

	answers := make(map[string]interface{})

	for i, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		questionText, _ := question["question"].(string)
		header, _ := question["header"].(string)

		if header != "" {
			fmt.Printf("\n%s[%s]%s\n", ColorCyan, header, ColorReset)
		}
		fmt.Printf("%s\n", questionText)

		options, ok := question["options"].([]interface{})
		if !ok || len(options) == 0 {
			continue
		}

		fmt.Printf("\nOptions:\n")
		for j, opt := range options {
			option, ok := opt.(map[string]interface{})
			if !ok {
				continue
			}
			label, _ := option["label"].(string)
			desc, _ := option["description"].(string)
			if desc != "" {
				fmt.Printf("  %s%d%s. %s - %s\n", ColorGreen, j+1, ColorReset, label, desc)
			} else {
				fmt.Printf("  %s%d%s. %s\n", ColorGreen, j+1, ColorReset, label)
			}
		}

		fmt.Printf("\n%sSelect option (1-%d) or type label: %s", ColorGreen, len(options), ColorReset)

		type result struct {
			response string
			err      error
		}
		resultChan := make(chan result, 1)

		go func() {
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			response = strings.TrimSpace(response)
			resultChan <- result{response: response, err: err}
		}()

		select {
		case <-ctx.Done():
			return agentboot.AskResult{ID: req.ID, Approved: false}, ctx.Err()
		case r := <-resultChan:
			if r.err != nil {
				return agentboot.AskResult{ID: req.ID, Approved: false}, r.err
			}

			var selectedIndex int = -1
			var selectedLabel string

			var num int
			if _, err := fmt.Sscanf(r.response, "%d", &num); err == nil {
				if num >= 1 && num <= len(options) {
					selectedIndex = num - 1
				}
			}

			if selectedIndex < 0 {
				for j, opt := range options {
					if option, ok := opt.(map[string]interface{}); ok {
						if label, ok := option["label"].(string); ok {
							if strings.EqualFold(label, r.response) {
								selectedIndex = j
								break
							}
						}
					}
				}
			}

			if selectedIndex < 0 {
				selectedLabel = r.response
			} else if selectedIndex >= 0 && selectedIndex < len(options) {
				if option, ok := options[selectedIndex].(map[string]interface{}); ok {
					if label, ok := option["label"].(string); ok {
						selectedLabel = label
					}
				}
			}

			answers[fmt.Sprintf("%d", i)] = selectedLabel
		}
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

// OnError implements agentboot.MessageStreamer
func (h *StdinHandler) OnError(err error) {
	fmt.Printf("[Error]: %v\n", err)
}

// OnMessage implements agentboot.MessageStreamer
func (h *StdinHandler) OnMessage(msg interface{}) error {
	return nil
}

// OnComplete implements agentboot.CompletionCallback
func (h *StdinHandler) OnComplete(result *agentboot.CompletionResult) {
	if h.Debug {
		status := "completed"
		if !result.Success {
			status = "failed"
		}
		fmt.Printf("[Complete]: %s\n", status)
	}
}

var _ agentboot.MessageHandler = (*StdinHandler)(nil)

// Example 1: Simple query with Launcher
func exampleSimpleQuery() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 1: Simple Query ===")

	result, err := launcher.Execute(ctx, "Say hello in one word", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Result: %s\n", result.Output)
	fmt.Printf("Duration: %v\n", result.Duration)
}

// Example 2: Stream format query
func exampleStreamQuery() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 2: Stream Format Query ===")

	result, err := launcher.Execute(ctx, "What is 2+2? Answer with just the number.", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Text Output: %s\n", result.TextOutput())
	fmt.Printf("Status: %s\n", result.GetStatus())
}

// Example 3: With model selection
func exampleWithModel() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 3: With Model Selection ===")

	result, err := launcher.Execute(ctx, "Explain Go channels in one sentence", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
		//Model:        "claude-sonnet-4-6",
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Result: %s\n", result.Output)
}

// Example 4: Resume conversation
func exampleResume() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	// In real usage, you'd get sessionID from a previous execution
	sessionID := "your-session-id-here"

	fmt.Println("=== Example 4: Resume Conversation ===")

	result, err := launcher.Execute(ctx, "Continue our conversation about Go", agentboot.ExecutionOptions{
		Handler:       handler,
		ProjectPath:   "/tmp",
		OutputFormat:  agentboot.OutputFormatText,
		SessionID:     sessionID,
		Resume:        true,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Result: %s\n", result.Output)
	fmt.Printf("Session ID: %s\n", result.GetSessionID())
}

// Example 5: Continue conversation
func exampleContinue() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{
		ContinueConversation: true,
	})
	handler := NewStdinHandler()

	fmt.Println("=== Example 5: Continue Conversation ===")

	result, err := launcher.Execute(ctx, "What were we discussing?", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Result: %s\n", result.Output)
}

// Example 6: With tool filtering
func exampleWithToolFiltering() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 6: With Tool Filtering ===")

	result, err := launcher.Execute(ctx, "Read the files in current directory", agentboot.ExecutionOptions{
		Handler:         handler,
		ProjectPath:     "/tmp",
		OutputFormat:    agentboot.OutputFormatStreamJSON,
		AllowedTools:    []string{"Read", "Bash"},
		DisallowedTools: []string{"Write", "Edit"},
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Text Output: %s\n", result.TextOutput())
}

// Example 7: With timeout
func exampleWithTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 7: With Timeout ===")

	result, err := launcher.Execute(ctx, "Count to 100 slowly", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      30 * time.Second,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Result: %s\n", result.Output)
}

// Example 8: With AskUserQuestion handling
func exampleAskUserQuestion() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	launcher := claude.NewLauncher(claude.Config{})
	handler := NewStdinHandler()

	fmt.Println("=== Example 8: AskUserQuestion Handling ===")

	result, err := launcher.Execute(ctx, "Ask me what I want to help with today", agentboot.ExecutionOptions{
		Handler:      handler,
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		log.Printf("Execute failed: %v", err)
		return
	}

	fmt.Printf("Text Output: %s\n", result.TextOutput())
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run query_example.go <example>")
		fmt.Println("Examples:")
		fmt.Println("  1 - Simple query")
		fmt.Println("  2 - Stream format query")
		fmt.Println("  3 - With model selection")
		fmt.Println("  4 - Resume conversation")
		fmt.Println("  5 - Continue conversation")
		fmt.Println("  6 - With tool filtering")
		fmt.Println("  7 - With timeout")
		fmt.Println("  8 - AskUserQuestion handling")
		os.Exit(1)
	}

	example := os.Args[1]

	switch example {
	case "1":
		exampleSimpleQuery()
	case "2":
		exampleStreamQuery()
	case "3":
		exampleWithModel()
	case "4":
		exampleResume()
	case "5":
		exampleContinue()
	case "6":
		exampleWithToolFiltering()
	case "7":
		exampleWithTimeout()
	case "8":
		exampleAskUserQuestion()
	default:
		fmt.Printf("Unknown example: %s\n", example)
		os.Exit(1)
	}
}