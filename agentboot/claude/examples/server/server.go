package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

// Server handles Claude interaction with simplified input/output
// It uses stream-json input format internally
type Server struct {
	launcher   *claude.QueryLauncher
	model      string
	cwd        string
	allowTools []string
	debug      bool
}

// NewServer creates a new server instance
func NewServer() *Server {
	return &Server{
		launcher:   claude.NewQueryLauncher(claude.Config{}),
		model:      "",
		cwd:        "",
		allowTools: nil, // Allow all tools by default
		debug:      false,
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

// SetAllowedTools sets which tools are allowed
func (s *Server) SetAllowedTools(tools []string) {
	s.allowTools = tools
}

// SetDebug enables debug output
func (s *Server) SetDebug(debug bool) {
	s.debug = debug
}

// ProcessQuery processes a single user query using stream-json input
func (s *Server) ProcessQuery(ctx context.Context, userPrompt string, continueConversation bool) (string, error) {
	// Build stream prompt with the user message
	// The server automatically wraps the user input in the correct stream-json format
	builder := claude.NewStreamPromptBuilder()
	builder.AddUserMessage(userPrompt)

	// Define tool permission callback
	// Response format according to Claude CLI Agent Protocol:
	// - Allow: {"behavior": "allow", "updatedInput": {...}}
	// - Deny:  {"behavior": "deny", "message": "reason"}
	//
	// This callback sends permission requests to the main loop for user interaction.
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		// Always log permission requests for verification
		log.Printf("[Permission Request] Tool: %s", toolName)
		if s.debug {
			log.Printf("[Permission Request] Input: %+v", input)
		}

		// Check if tool is explicitly allowed
		if s.allowTools != nil {
			allowed := false
			for _, t := range s.allowTools {
				if t == toolName {
					allowed = true
					break
				}
			}
			if !allowed {
				// Deny with message
				log.Printf("[Permission] DENIED: tool %s is not in allowed list", toolName)
				return map[string]interface{}{
					"behavior": "deny",
					"message":  fmt.Sprintf("tool %s is not allowed", toolName),
				}, nil
			}
		}

		// Auto-approve if tool is in allowed list
		if s.allowTools != nil {
			for _, t := range s.allowTools {
				if t == toolName {
					log.Printf("[Permission] AUTO-APPROVED: tool %s is in allowed list", toolName)
					return map[string]interface{}{
						"behavior":     "allow",
						"updatedInput": input,
					}, nil
				}
			}
		}

		// Ask user directly via stdin
		// Note: Main loop is blocked waiting for query result, so we can safely read from stdin
		fmt.Printf("\r%s[Tool Permission]%s Claude wants to use: %s%s\n", ColorYellow, ColorReset, ColorCyan, toolName)
		if cmd, ok := input["command"].(string); ok {
			fmt.Printf("%sCommand%s: %s\n", ColorCyan, ColorReset, cmd)
		} else if s.debug {
			fmt.Printf("%sInput%s: %+v\n", ColorCyan, ColorReset, input)
		}
		fmt.Printf("%sAllow?%s (y=yes/n=no/a=always): ", ColorGreen, ColorReset)

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		switch response {
		case "y", "yes":
			log.Printf("[Permission] User APPROVED tool: %s", toolName)
			return map[string]interface{}{
				"behavior":     "allow",
				"updatedInput": input,
			}, nil
		case "a", "always", "al":
			// Add to allowed tools
			if s.allowTools == nil {
				s.allowTools = []string{}
			}
			s.allowTools = append(s.allowTools, toolName)
			log.Printf("[Permission] User APPROVED tool: %s (added to allowed list)", toolName)
			fmt.Printf("%s-> Tool '%s' added to allowed list%s\n", ColorGreen, toolName, ColorReset)
			return map[string]interface{}{
				"behavior":     "allow",
				"updatedInput": input,
			}, nil
		case "n", "no":
			log.Printf("[Permission] User DENIED tool: %s", toolName)
			return map[string]interface{}{
				"behavior": "deny",
				"message":  "User denied this tool use",
			}, nil
		default:
			log.Printf("[Permission] User DENIED tool: %s (invalid response)", toolName)
			return map[string]interface{}{
				"behavior": "deny",
				"message":  fmt.Sprintf("Invalid response. Please answer with 'y' (yes), 'n' (no), or 'a' (always)"),
			}, nil
		}
	}

	// Build query options
	options := &claude.QueryOptionsConfig{
		CanCallTool:          canCallTool,
		ContinueConversation: continueConversation,
	}

	if s.model != "" {
		options.Model = s.model
	}
	if s.cwd != "" {
		options.CWD = s.cwd
	}

	// Create and execute query (uses stream-json input format internally)
	// --permission-prompt-tool stdio is automatically added for stream input
	query, err := s.launcher.Query(ctx, claude.QueryConfig{
		Prompt:  builder.Close(),
		Options: options,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create query: %w", err)
	}
	defer query.Close()

	// Collect response
	var response strings.Builder
	var assistantText strings.Builder

messageLoop:
	for {
		msg, ok := query.Next()
		if !ok {
			log.Printf("[ProcessQuery] No more messages")
			break
		}

		log.Printf("[ProcessQuery] Received message type: %s", msg.Type)

		if s.debug {
			bs, err := json.Marshal(msg)
			if err != nil {
				log.Printf("[Debug] Failed to marshal msg: %v", err)
			} else {
				log.Printf("[Debug] Message: %s", bs)
			}
		}

		switch msg.Type {
		case "system":
			if s.debug {
				log.Printf("[System] Session: %s", msg.SessionID)
			}
		case "assistant":
			// Extract text from assistant message
			if msg.Message != nil {
				if content := extractTextContent(msg.Message); content != "" {
					assistantText.WriteString(content)
					response.WriteString(content)
				}
			}
		case "user":
			// User messages contain tool results or other user content
			// These are handled internally by Claude CLI, but we can log them for debugging
			if s.debug {
				log.Printf("[User] User message received (tool result or user input)")
			}
		case "tool_use":
			log.Printf("[Tool Use] Tool: %v", msg.Request)
			// Tool use is handled internally by Claude CLI
		case "tool_result":
			log.Printf("[Tool Result] Tool result received")
			// Tool result is handled internally by Claude CLI
		case "result":
			log.Printf("[Result] Result: %s", msg.Result)
			if msg.Result != "" {
				// Always append result - it contains the final response
				response.WriteString(msg.Result)
			}
			// Result message indicates the turn is complete
			// Use labeled break to exit the for loop
			break messageLoop
		case "control_request", "control_response", "control_cancel_request":
			// These are handled internally by the Query
			log.Printf("[Control] %s message handled internally", msg.Type)
		default:
			log.Printf("[Unknown] Unknown message type: %s", msg.Type)
		}
	}

	log.Printf("[ProcessQuery] Returning response length: %d", response.Len())
	return response.String(), nil
}

// extractTextContent extracts text content from a message
func extractTextContent(msg map[string]interface{}) string {
	if msg == nil {
		return ""
	}

	content, ok := msg["content"].([]interface{})
	if !ok {
		// Try direct string content
		if str, ok := msg["content"].(string); ok {
			return str
		}
		return ""
	}

	var result strings.Builder
	for _, c := range content {
		if block, ok := c.(map[string]interface{}); ok {
			if blockType, ok := block["type"].(string); ok && blockType == "text" {
				if text, ok := block["text"].(string); ok {
					result.WriteString(text)
				}
			}
		}
	}

	return result.String()
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	Response string
	Error    error
}

// ProcessQueryAsync processes a query asynchronously, sending results to the provided channel
// The main loop can continue handling permission requests while the query runs
func (s *Server) ProcessQueryAsync(ctx context.Context, userPrompt string, continueConversation bool, resultChan chan<- QueryResult, interruptFunc context.CancelFunc) {
	go func() {
		response, err := s.ProcessQuery(ctx, userPrompt, continueConversation)
		resultChan <- QueryResult{Response: response, Error: err}
		interruptFunc()
	}()
}

// Run starts the server's interactive loop
func (s *Server) Run(ctx context.Context) error {
	fmt.Printf("%sClaude Interactive Server (stream-json mode)%s\n", ColorCyan, ColorReset)
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
		s.ProcessQueryAsync(queryCtx, userInput, conversationActive, resultChan, cancel)

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
		case "--allow-tools":
			if i+1 < len(args) {
				i++
				tools := strings.Split(args[i], ",")
				server.SetAllowedTools(tools)
			}
		case "--debug", "-d":
			server.SetDebug(true)
		case "--help", "-h":
			fmt.Println("Claude Interactive Server")
			fmt.Println("\nA server that simplifies Claude interaction using stream-json input.")
			fmt.Println("\nUsage: go run server.go [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  --model, -m <model>       Set the model to use")
			fmt.Println("  --cwd, -c <directory>     Set working directory")
			fmt.Println("  --allow-tools <tools>     Comma-separated list of allowed tools")
			fmt.Println("  --debug, -d               Enable debug output")
			fmt.Println("  --help, -h                Show this help message")
			fmt.Println("\nInteractive commands:")
			fmt.Println("  debug                     Toggle debug mode")
			fmt.Println("  new                       Start a new conversation")
			fmt.Println("  quit, exit, q             Exit the server")
			fmt.Println("\nFeatures:")
			fmt.Println("  - Automatically wraps user input in correct stream-json format")
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
