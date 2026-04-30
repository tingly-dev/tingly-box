package smart_guide

import (
	"context"

	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

// Handoff Tool (Hidden for now)
// ============================================================================

// HandoffParams defines the parameters for handoff_to_cc tool
type HandoffParams struct{}

// HandoffToCCTool provides handoff to Claude Code
// Note: Currently not registered, kept for future use
type HandoffToCCTool struct{}

// NewHandoffToCCTool creates a new handoff tool
func NewHandoffToCCTool() *HandoffToCCTool {
	return &HandoffToCCTool{}
}

// Name returns the tool name
func (t *HandoffToCCTool) Name() string {
	return "handoff_to_cc"
}

// Description returns the tool description
func (t *HandoffToCCTool) Description() string {
	return "Hand off control to Claude Code (@cc) for coding tasks. Use this when the user is ready to start coding."
}

// Call implements the tool interface
func (t *HandoffToCCTool) Call(ctx context.Context, params HandoffParams) (*tool.ToolResponse, error) {
	return tool.TextResponse("HANDOFF_TO_CC"), nil
}

// ============================================================================
