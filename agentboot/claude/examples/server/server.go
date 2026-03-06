package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Color codes for output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

// StdinHandler implements MessageHandler using stdin/stdout.
// This is a simple implementation for the example server.
type StdinHandler struct {
	Debug bool
}

// NewStdinHandler creates a new StdinHandler
func NewStdinHandler() *StdinHandler {
	return &StdinHandler{}
}

// OnApproval implements agentboot.ApprovalHandler
func (h *StdinHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// AskUserQuestion is a special case - it presents options for user to select,
	// not a simple yes/no permission approval
	if req.ToolName == "AskUserQuestion" {
		return h.handleAskUserQuestionApproval(ctx, req)
	}

	// Display permission request
	fmt.Printf("\r%s[Tool Permission]%s Claude wants to use: %s%s\n",
		ColorYellow, ColorReset, ColorCyan, req.ToolName)

	// Show relevant input details
	if cmd, ok := req.Input["command"].(string); ok {
		fmt.Printf("%sCommand%s: %s\n", ColorCyan, ColorReset, cmd)
	} else if h.Debug {
		fmt.Printf("%sInput%s: %+v\n", ColorCyan, ColorReset, req.Input)
	}

	fmt.Printf("%sAllow?%s (y=yes/n=no/a=always): ", ColorGreen, ColorReset)

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
		case "a", "always", "al":
			return agentboot.PermissionResult{
				Approved:     true,
				UpdatedInput: req.Input,
				Remember:     true,
			}, nil
		case "n", "no":
			return agentboot.PermissionResult{Approved: false}, nil
		default:
			// Invalid response - treat as deny
			return agentboot.PermissionResult{Approved: false}, nil
		}
	}
}

// handleAskUserQuestionApproval handles AskUserQuestion as a permission request
// This presents options for user selection instead of simple y/n
func (h *StdinHandler) handleAskUserQuestionApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		// No questions - auto approve
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Display the questions and options
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

		// Show options
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

		// Prompt for selection
		fmt.Printf("\n%sSelect option (1-%d) or type label: %s", ColorGreen, len(options), ColorReset)

		// Read user input
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

		// Wait for input or context cancellation
		select {
		case <-ctx.Done():
			return agentboot.PermissionResult{Approved: false}, ctx.Err()
		case r := <-resultChan:
			if r.err != nil {
				return agentboot.PermissionResult{Approved: false}, r.err
			}

			// Try to parse as number
			var selectedIndex int = -1
			var selectedLabel string

			// Try numeric parsing
			var num int
			if _, err := fmt.Sscanf(r.response, "%d", &num); err == nil {
				if num >= 1 && num <= len(options) {
					selectedIndex = num - 1
				}
			}

			// If not a valid number, try matching by label
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

			// If still not found, use the raw input as label
			if selectedIndex < 0 {
				selectedLabel = r.response
			} else if selectedIndex >= 0 && selectedIndex < len(options) {
				if option, ok := options[selectedIndex].(map[string]interface{}); ok {
					if label, ok := option["label"].(string); ok {
						selectedLabel = label
					}
				}
			}

			// Store answer
			answers[fmt.Sprintf("%d", i)] = selectedLabel
		}
	}

	// Build updated input with answers
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
// This handles user questions that require selection or input (NOT tool permissions).
// For tool permission approval, see OnApproval method.
func (h *StdinHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	// AskUserQuestion is the most common ask type - it presents options for user to select
	if req.ToolName == "AskUserQuestion" {
		return h.handleAskUserQuestion(ctx, req)
	}

	// For other ask types, check if there are options to select from
	if questions, ok := req.Input["questions"].([]interface{}); ok && len(questions) > 0 {
		return h.handleAskUserQuestion(ctx, req)
	}

	// For simple text input requests
	if req.Type == "text_input" || req.Message != "" {
		return h.handleTextInput(ctx, req)
	}

	// Default: simple confirmation (y/n)
	return h.handleConfirmation(ctx, req)
}

// handleTextInput handles text input requests
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

// handleConfirmation handles simple yes/no confirmation
func (h *StdinHandler) handleConfirmation(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	if req.Message != "" {
		fmt.Printf("\r%s[Confirmation]%s %s\n", ColorYellow, ColorReset, req.Message)
	}
	fmt.Printf("%sConfirm?%s (y/n): ", ColorGreen, ColorReset)

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

	select {
	case <-ctx.Done():
		return agentboot.AskResult{ID: req.ID, Approved: false}, ctx.Err()
	case r := <-resultChan:
		if r.err != nil {
			return agentboot.AskResult{ID: req.ID, Approved: false}, r.err
		}
		approved := r.response == "y" || r.response == "yes"
		return agentboot.AskResult{ID: req.ID, Approved: approved}, nil
	}
}

// handleAskUserQuestion handles AskUserQuestion tool with multi-option selection
func (h *StdinHandler) handleAskUserQuestion(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return agentboot.AskResult{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Display the questions and options
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

		// Show options
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

		// Prompt for selection
		fmt.Printf("\n%sSelect option (1-%d) or type label: %s", ColorGreen, len(options), ColorReset)

		// Read user input
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

		// Wait for input or context cancellation
		select {
		case <-ctx.Done():
			return agentboot.AskResult{ID: req.ID, Approved: false}, ctx.Err()
		case r := <-resultChan:
			if r.err != nil {
				return agentboot.AskResult{ID: req.ID, Approved: false}, r.err
			}

			// Try to parse as number
			var selectedIndex int = -1
			var selectedLabel string

			// Try numeric parsing
			var num int
			if _, err := fmt.Sscanf(r.response, "%d", &num); err == nil {
				if num >= 1 && num <= len(options) {
					selectedIndex = num - 1
				}
			}

			// If not a valid number, try matching by label
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

			// If still not found, use the raw input as label
			if selectedIndex < 0 {
				selectedLabel = r.response
			} else if selectedIndex >= 0 && selectedIndex < len(options) {
				if option, ok := options[selectedIndex].(map[string]interface{}); ok {
					if label, ok := option["label"].(string); ok {
						selectedLabel = label
					}
				}
			}

			// Store answer
			answers[fmt.Sprintf("%d", i)] = selectedLabel
		}
	}

	// Build updated input with answers
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
	fmt.Printf("%s[Error]%s: %v\n", ColorRed, ColorReset, err)
}

// OnMessage implements agentboot.MessageStreamer - collects messages for output
func (h *StdinHandler) OnMessage(msg interface{}) error {
	// For this simple example, we don't need to process messages here
	// The Launcher will collect results via ExecuteWithTimeout
	return nil
}

// OnComplete implements agentboot.CompletionCallback
func (h *StdinHandler) OnComplete(result *agentboot.CompletionResult) {
	if h.Debug {
		status := "completed"
		if !result.Success {
			status = "failed"
		}
		fmt.Printf("%s[Complete]%s: %s\n", ColorCyan, ColorReset, status)
	}
}

// Ensure StdinHandler implements required interfaces
var _ agentboot.MessageHandler = (*StdinHandler)(nil)

// Server handles Claude interaction with simplified input/output
type Server struct {
	launcher *claude.Launcher
	handler  *StdinHandler
	model    string
	cwd      string
	debug    bool
}

// NewServer creates a new server instance
func NewServer() *Server {
	return &Server{
		launcher: claude.NewLauncher(claude.Config{}),
		handler:  NewStdinHandler(),
	}
}

// SetModel sets the model to use
func (s *Server) SetModel(model string) {
	s.model = model
}

// SetCWD sets the working directory
func (s *Server) SetCWD(cwd string) {
	s.cwd = cwd
}

// SetDebug enables debug output
func (s *Server) SetDebug(debug bool) {
	s.debug = debug
	s.handler.Debug = debug
}

// QueryAgent processes a single user query
func (s *Server) QueryAgent(ctx context.Context, userPrompt string, continueConversation bool) (string, error) {
	// Build execution options
	opts := agentboot.ExecutionOptions{
		Handler:    s.handler,
		Model:      s.model,
		ProjectPath: s.cwd,
	}

	// Use stream-json format for proper message handling
	opts.OutputFormat = agentboot.OutputFormatStreamJSON

	// Execute using Launcher
	result, err := s.launcher.Execute(ctx, userPrompt, opts)
	if err != nil {
		return "", fmt.Errorf("failed to execute: %w", err)
	}

	// Extract text output from result
	return result.TextOutput(), nil
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	Response string
	Error    error
}

// QueryAgentAsync processes a query asynchronously
func (s *Server) QueryAgentAsync(ctx context.Context, userPrompt string, continueConversation bool, resultChan chan<- QueryResult, interruptFunc context.CancelFunc) {
	go func() {
		response, err := s.QueryAgent(ctx, userPrompt, continueConversation)
		resultChan <- QueryResult{Response: response, Error: err}
		interruptFunc()
	}()
}

// Run starts the server's interactive loop
func (s *Server) Run(ctx context.Context) error {
	fmt.Printf("%sClaude Interactive Server%s\n", ColorCyan, ColorReset)
	fmt.Printf("%sType your message and press Enter. Type 'quit' or 'exit' to quit.%s\n", ColorYellow, ColorReset)
	fmt.Printf("%sPress Ctrl-C to exit.%s\n\n", ColorYellow, ColorReset)

	reader := bufio.NewReader(os.Stdin)
	conversationActive := false

	// Show prompt BEFORE waiting for input
	prompt := fmt.Sprintf("%sYou%s> ", ColorGreen, ColorReset)

	for {
		if conversationActive {
			prompt = fmt.Sprintf("%sYou%s> ", ColorBlue, ColorReset)
		}
		fmt.Print(prompt)

		// Read user input
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		userInput := strings.TrimSpace(line)

		if userInput == "" {
			continue
		}

		// Check for exit commands
		if userInput == "quit" || userInput == "exit" || userInput == "q" {
			fmt.Printf("%sGoodbye!%s\n", ColorYellow, ColorReset)
			return nil
		}

		// Check for debug toggle
		if userInput == "debug" {
			s.debug = !s.debug
			s.handler.Debug = s.debug
			fmt.Printf("%sDebug mode: %v%s\n", ColorYellow, s.debug, ColorReset)
			continue
		}

		// Check for new conversation
		if userInput == "new" {
			conversationActive = false
			fmt.Printf("%sStarted new conversation%s\n", ColorYellow, ColorReset)
			continue
		}

		// Create timeout context for this query
		queryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		// Channel for query result
		resultChan := make(chan QueryResult, 1)

		// Start query asynchronously so permission prompts can read from stdin
		s.QueryAgentAsync(queryCtx, userInput, conversationActive, resultChan, cancel)

		// Wait for result
		result := <-resultChan
		cancel()

		if result.Error != nil {
			fmt.Printf("%sError: %v%s\n", ColorRed, result.Error, ColorReset)
			continue
		}

		// Display response
		fmt.Printf("\n%sClaude%s:\n%s%s%s\n\n", ColorPurple, ColorReset, ColorWhite, result.Response, ColorReset)

		// Continue the conversation
		conversationActive = true
	}
}

func main() {
	// Parse command line arguments
	server := NewServer()

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model", "-m":
			if i+1 < len(args) {
				i++
				server.SetModel(args[i])
			}
		case "--cwd", "-c":
			if i+1 < len(args) {
				i++
				server.SetCWD(args[i])
			}
		case "--debug", "-d":
			server.SetDebug(true)
		case "--help", "-h":
			fmt.Println("Claude Interactive Server")
			fmt.Println("\nA server that simplifies Claude interaction.")
			fmt.Println("\nUsage: go run server.go [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  --model, -m <model>       Set the model to use")
			fmt.Println("  --cwd, -c <directory>     Set working directory")
			fmt.Println("  --debug, -d               Enable debug output")
			fmt.Println("  --help, -h                Show this help message")
			fmt.Println("\nInteractive commands:")
			fmt.Println("  debug                     Toggle debug mode")
			fmt.Println("  new                       Start a new conversation")
			fmt.Println("  quit, exit, q             Exit the server")
			fmt.Println("\nFeatures:")
			fmt.Println("  - Supports multi-turn conversations")
			fmt.Println("  - Auto-approves allowed tools")
			fmt.Println("  - Simplified output (just shows Claude's response)")
			os.Exit(0)
		}
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Printf("\n%sInterrupted. Goodbye!%s\n", ColorYellow, ColorReset)
		os.Exit(0)
	}()

	// Run the server
	if err := server.Run(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
