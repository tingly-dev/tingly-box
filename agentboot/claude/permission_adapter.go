package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
)

// PermissionAdapter converts permission.Handler to CanCallToolCallback
// It bridges the gap between the Claude SDK callback format and the
// permission.Handler interface.
type PermissionAdapter struct {
	handler   permission.Handler
	sessionID string
	agentType agentboot.AgentType
}

// PermissionAdapterOption is a functional option for PermissionAdapter
type PermissionAdapterOption func(*PermissionAdapter)

// WithSessionID sets the session ID for the adapter
func WithSessionID(sessionID string) PermissionAdapterOption {
	return func(a *PermissionAdapter) {
		a.sessionID = sessionID
	}
}

// WithAgentType sets the agent type for the adapter
func WithAgentType(agentType agentboot.AgentType) PermissionAdapterOption {
	return func(a *PermissionAdapter) {
		a.agentType = agentType
	}
}

// NewPermissionAdapter creates a new PermissionAdapter
func NewPermissionAdapter(handler permission.Handler, opts ...PermissionAdapterOption) *PermissionAdapter {
	a := &PermissionAdapter{
		handler:   handler,
		sessionID: "",
		agentType: agentboot.AgentTypeClaude,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// AsCallback returns a CanCallToolCallback that uses the permission handler
// The callback returns Claude CLI Agent Protocol compliant responses:
//   - Allow: {"behavior": "allow", "updatedInput": {...}}
//   - Deny:  {"behavior": "deny", "message": "reason"}
func (a *PermissionAdapter) AsCallback() CanCallToolCallback {
	return func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error) {
		// Create permission request
		req := agentboot.PermissionRequest{
			RequestID: uuid.New().String(),
			AgentType: a.agentType,
			ToolName:  toolName,
			Input:     input,
			Timestamp: time.Now(),
			SessionID: a.sessionID,
		}

		// Call handler
		result, err := a.handler.CanUseTool(ctx, req)
		if err != nil {
			return denyResponse(fmt.Sprintf("Permission check failed: %v", err)), nil
		}

		if result.Approved {
			return allowResponse(input), nil
		}

		return denyResponse(result.Reason), nil
	}
}

// allowResponse creates an allow response for Claude CLI
func allowResponse(input map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"behavior":     "allow",
		"updatedInput": input,
	}
}

// denyResponse creates a deny response for Claude CLI
func denyResponse(reason string) map[string]interface{} {
	if reason == "" {
		reason = "Permission denied"
	}
	return map[string]interface{}{
		"behavior": "deny",
		"message":  reason,
	}
}

// SimplePermissionAdapter is a simpler adapter that directly uses a callback
// without the full permission.Handler interface. Useful for simple use cases.
type SimplePermissionAdapter struct {
	// Whitelist of tool names that are automatically approved
	Whitelist []string

	// Blacklist of tool names that are automatically denied
	Blacklist []string

	// UserPrompter is called for tools not in whitelist/blacklist
	// If nil, non-whitelisted tools are denied
	UserPrompter permission.UserPrompter

	// Debug enables verbose logging
	Debug bool

	// SessionID for tracking
	SessionID string
}

// NewSimplePermissionAdapter creates a new SimplePermissionAdapter
func NewSimplePermissionAdapter() *SimplePermissionAdapter {
	return &SimplePermissionAdapter{}
}

// AsCallback returns a CanCallToolCallback
func (a *SimplePermissionAdapter) AsCallback() CanCallToolCallback {
	return func(ctx context.Context, toolName string, input map[string]interface{}, opts CallToolOptions) (map[string]interface{}, error) {
		// Check blacklist first
		for _, blacklisted := range a.Blacklist {
			if blacklisted == toolName {
				return denyResponse(fmt.Sprintf("Tool '%s' is blacklisted", toolName)), nil
			}
		}

		// Check whitelist
		for _, whitelisted := range a.Whitelist {
			if whitelisted == toolName {
				return allowResponse(input), nil
			}
		}

		// If no user prompter, deny
		if a.UserPrompter == nil {
			return denyResponse(fmt.Sprintf("Tool '%s' is not in whitelist", toolName)), nil
		}

		// Ask user
		req := agentboot.PermissionRequest{
			RequestID: uuid.New().String(),
			AgentType: agentboot.AgentTypeClaude,
			ToolName:  toolName,
			Input:     input,
			Timestamp: time.Now(),
			SessionID: a.SessionID,
		}

		approved, remember, err := a.UserPrompter.PromptPermission(ctx, req)
		if err != nil {
			return denyResponse(fmt.Sprintf("Permission prompt failed: %v", err)), nil
		}

		if remember && approved {
			// Add to whitelist
			a.Whitelist = append(a.Whitelist, toolName)
		}

		if approved {
			return allowResponse(input), nil
		}

		return denyResponse("User denied this tool use"), nil
	}
}
