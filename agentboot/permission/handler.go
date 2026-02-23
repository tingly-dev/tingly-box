package permission

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Handler handles permission requests from agents
type Handler interface {
	// CanUseTool checks if a tool can be used
	CanUseTool(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error)

	// SetMode sets the permission mode for a session/chat
	SetMode(scopeID string, mode agentboot.PermissionMode) error

	// GetMode gets the current permission mode
	GetMode(scopeID string) (agentboot.PermissionMode, error)

	// SubmitDecision submits a permission decision (for manual mode)
	SubmitDecision(requestID string, approved bool, reason string) error

	// GetPendingRequests returns all pending permission requests
	GetPendingRequests() []agentboot.PermissionRequest

	// RecordDecision records a permission decision for learning
	RecordDecision(req agentboot.PermissionRequest, response agentboot.PermissionResponse) error
}

// NewDefaultHandler creates a new permission handler with the given config
func NewDefaultHandler(config agentboot.PermissionConfig) Handler {
	return newDefaultHandler(config)
}
