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
	// Note: --permission-prompt-tool stdio is automatically added for stream input
	// which handles actual permission requests via stdio. This callback is
	// kept for validation purposes and can be used for additional filtering.
	//
	// Response format according to Claude CLI Agent Protocol:
	// - Allow: {"behavior": "allow", "updatedInput": {...}}
	// - Deny:  {"behavior": "deny", "message": "reason"}
	canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
		if s.debug {
			log.Printf("[Tool Permission] %s: %v", toolName, input)
		}

		// Check if tool is allowed
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
				return map[string]interface{}{
					"behavior": "deny",
					"message":  fmt.Sprintf("tool %s is not allowed", toolName),
				}, nil
			}
		}

		// Allow with updatedInput (required by protocol)
		return map[string]interface{}{
			"behavior":     "allow",
			"updatedInput": input,
		}, nil
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

	for {
		msg, ok := query.Next()
		if !ok {
			break
		}

		if s.debug {
			log.Printf("[Debug] Message type: %s", msg.Type)
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
		case "tool_use":
			if s.debug {
				if name, ok := msg.Request["name"].(string); ok {
					log.Printf("[Tool Use] %s", name)
				}
			}
		case "tool_result":
			if s.debug {
				log.Printf("[Tool Result]")
			}
		case "result":
			if msg.Result != "" && assistantText.Len() == 0 {
				// If we haven't captured assistant text yet, use the result
				response.WriteString(msg.Result)
			}
		}
	}

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

// Run starts the server's interactive loop
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%sClaude Interactive Server (stream-json mode)%s\n", ColorCyan, ColorReset)
	fmt.Printf("%sType your message and press Enter. Type 'quit' or 'exit' to quit.%s\n", ColorYellow, ColorReset)
	fmt.Printf("%sPress Ctrl-C to exit.%s\n\n", ColorYellow, ColorReset)

	conversationActive := false

	// Channel to signal scanner to stop
	stopScan := make(chan struct{})
	defer close(stopScan)

	// Goroutine to handle input
	inputChan := make(chan string)
	errChan := make(chan error, 1)

	go func() {
		for {
			select {
			case <-stopScan:
				return
			default:
				if !scanner.Scan() {
					errChan <- scanner.Err()
					return
				}
				userInput := strings.TrimSpace(scanner.Text())
				inputChan <- userInput
			}
		}
	}()

	for {
		// Show prompt BEFORE waiting for input
		prompt := fmt.Sprintf("%sYou%s> ", ColorGreen, ColorReset)
		if conversationActive {
			prompt = fmt.Sprintf("%sYou%s> ", ColorBlue, ColorReset)
		}
		fmt.Print(prompt)

		select {
		case <-ctx.Done():
			fmt.Printf("\n%sInterrupted. Goodbye!%s\n", ColorYellow, ColorReset)
			return nil

		case err := <-errChan:
			return err

		case userInput := <-inputChan:
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

			// Process the query
			response, err := s.ProcessQuery(queryCtx, userInput, conversationActive)
			cancel()

			if err != nil {
				fmt.Printf("%sError: %v%s\n", ColorRed, err, ColorReset)
				continue
			}

			// Display response
			fmt.Printf("\n%sClaude%s:\n%s%s%s\n\n", ColorPurple, ColorReset, ColorWhite, response, ColorReset)

			// Continue the conversation
			conversationActive = true
		}
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
