package permission

import (
	"github.com/tingly-dev/tingly-box/agentboot"
)

// UserResponse represents a user's response to a permission prompt
type UserResponse struct {
	// Type indicates the response type: "button", "text", "selection"
	Type string

	// Data contains the raw response data (button callback or text input)
	Data string

	// Selections contains structured selections for multi-select scenarios
	// Key is typically the question index or ID, value is the selected option
	Selections map[string]interface{}
}

// ToolHandler defines the interface for tool-specific permission handling
// Each tool type can have its own handler to customize the prompt and response parsing
type ToolHandler interface {
	// CanHandle returns true if this handler can handle the given tool
	CanHandle(toolName string, input map[string]interface{}) bool

	// Description returns a human-readable description of this handler
	Description() string
}

// ToolPromptBuilder builds prompts for tool permission requests
type ToolPromptBuilder interface {
	ToolHandler

	// BuildPrompt creates the prompt message for this tool
	BuildPrompt(req agentboot.PermissionRequest) string
}

// ToolResponseParser parses user responses into permission results
type ToolResponseParser interface {
	ToolHandler

	// ParseResponse parses user response into PermissionResult
	// The result should include UpdatedInput if the tool requires structured responses
	ParseResponse(req agentboot.PermissionRequest, response UserResponse) (agentboot.PermissionResult, error)
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

// DefaultToolHandler is the fallback handler for tools without specific handlers
type DefaultToolHandler struct{}

// NewDefaultToolHandler creates a new DefaultToolHandler
func NewDefaultToolHandler() *DefaultToolHandler {
	return &DefaultToolHandler{}
}

// CanHandle returns true for all tools (acts as fallback)
func (h *DefaultToolHandler) CanHandle(toolName string, input map[string]interface{}) bool {
	return true
}

// Description returns the handler description
func (h *DefaultToolHandler) Description() string {
	return "Default handler for simple approve/deny decisions"
}

// BuildPrompt creates a simple permission prompt
func (h *DefaultToolHandler) BuildPrompt(req agentboot.PermissionRequest) string {
	return buildDefaultPrompt(req)
}

// ParseResponse parses a simple approve/deny response
func (h *DefaultToolHandler) ParseResponse(req agentboot.PermissionRequest, response UserResponse) (agentboot.PermissionResult, error) {
	return parseDefaultResponse(req, response)
}

// buildDefaultPrompt creates the default permission prompt text
func buildDefaultPrompt(req agentboot.PermissionRequest) string {
	text := "🔐 *Tool Permission Request*\n\n"
	text += "Tool: `" + req.ToolName + "`\n"

	// Show relevant input details
	if cmd, ok := req.Input["command"].(string); ok && cmd != "" {
		text += "Command: `" + truncatePromptText(cmd, 200) + "`\n"
	} else if filePath, ok := req.Input["file_path"].(string); ok && filePath != "" {
		text += "File: `" + filePath + "`\n"
	}

	if req.Reason != "" {
		text += "\nReason: " + req.Reason + "\n"
	}

	return text
}

// parseDefaultResponse parses standard allow/deny responses
func parseDefaultResponse(req agentboot.PermissionRequest, response UserResponse) (agentboot.PermissionResult, error) {
	switch response.Data {
	case "allow", "yes", "y", "1":
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
			Reason:       "User approved",
		}, nil
	case "always":
		return agentboot.PermissionResult{
			Approved:     true,
			UpdatedInput: req.Input,
			Remember:     true,
			Reason:       "User approved (always)",
		}, nil
	case "deny", "no", "n", "0":
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "User denied",
		}, nil
	default:
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "Unknown response",
		}, nil
	}
}

// truncatePromptText truncates text for prompt display
func truncatePromptText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
