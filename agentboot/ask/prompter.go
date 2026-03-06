package ask

import (
	"context"
)

// Prompter handles the actual user interaction
// Implementations: StdinPrompter, IMPrompter, NoOpPrompter
type Prompter interface {
	// Prompt sends a prompt to the user and returns the response
	Prompt(ctx context.Context, req Request) (Result, error)
}

// ToolHandler handles tool-specific ask requests
// Each tool type can have its own handler to customize the prompt and response parsing
type ToolHandler interface {
	// CanHandle returns true if this handler can handle the given tool
	CanHandle(toolName string, input map[string]interface{}) bool

	// Description returns a human-readable description of this handler
	Description() string
}

// ToolPromptBuilder builds prompts for tool requests
type ToolPromptBuilder interface {
	ToolHandler

	// BuildPrompt creates the prompt message for this tool
	BuildPrompt(req Request) string
}

// ToolResponseParser parses user responses into results
type ToolResponseParser interface {
	ToolHandler

	// ParseResponse parses user response into Result
	ParseResponse(req Request, response Response) (Result, error)
}

// ToolHandlerRegistry manages tool-specific handlers
type ToolHandlerRegistry struct {
	handlers []ToolHandler
}

// NewToolHandlerRegistry creates a new registry with default handlers
func NewToolHandlerRegistry() *ToolHandlerRegistry {
	registry := &ToolHandlerRegistry{
		handlers: make([]ToolHandler, 0),
	}

	// Register built-in handlers
	registry.Register(NewAskUserQuestionHandler())
	registry.Register(NewDefaultToolHandler())

	return registry
}

// Register adds a new handler to the registry
// Handlers are checked in reverse registration order (most recent first)
func (r *ToolHandlerRegistry) Register(h ToolHandler) {
	r.handlers = append(r.handlers, h)
}

// FindHandler finds a handler that can handle the given tool
func (r *ToolHandlerRegistry) FindHandler(toolName string, input map[string]interface{}) ToolHandler {
	// Check handlers in reverse order (most recently registered first)
	for i := len(r.handlers) - 1; i >= 0; i-- {
		if r.handlers[i].CanHandle(toolName, input) {
			return r.handlers[i]
		}
	}
	return nil
}

// FindPromptBuilder finds a handler that implements ToolPromptBuilder
func (r *ToolHandlerRegistry) FindPromptBuilder(toolName string, input map[string]interface{}) ToolPromptBuilder {
	h := r.FindHandler(toolName, input)
	if h == nil {
		return nil
	}
	if builder, ok := h.(ToolPromptBuilder); ok {
		return builder
	}
	return nil
}

// FindResponseParser finds a handler that implements ToolResponseParser
func (r *ToolHandlerRegistry) FindResponseParser(toolName string, input map[string]interface{}) ToolResponseParser {
	h := r.FindHandler(toolName, input)
	if h == nil {
		return nil
	}
	if parser, ok := h.(ToolResponseParser); ok {
		return parser
	}
	return nil
}