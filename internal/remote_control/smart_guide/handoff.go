package smart_guide

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// validAgents is the set of valid agent types for handoff
var validAgents = map[string]bool{
	AgentTypeTinglyBox:  true,
	AgentTypeClaudeCode: true,
	AgentTypeMock:       true,
}

// HandoffState represents the state during a handoff
type HandoffState struct {
	FromAgent        string    `json:"from_agent"`
	ToAgent          string    `json:"to_agent"`
	Timestamp        time.Time `json:"timestamp"`
	ProjectPath      string    `json:"project_path"`
	SessionID        string    `json:"session_id"`
	ChatID           string    `json:"chat_id"`
	PreservedContext []byte    `json:"preserved_context,omitempty"`
}

// HandoffResult represents the result of a handoff operation
type HandoffResult struct {
	Success   bool   `json:"success"`
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Message   string `json:"message"`
	NextHint  string `json:"next_hint"`
	Error     string `json:"error,omitempty"`
}

// HandoffManager handles handoff operations between agents
type HandoffManager struct {
	// Dependencies would be injected here
	// For now, we'll keep it simple
}

// NewHandoffManager creates a new handoff manager
func NewHandoffManager() *HandoffManager {
	return &HandoffManager{}
}

// ExecuteHandoff performs a handoff from one agent to another
func (hm *HandoffManager) ExecuteHandoff(ctx context.Context, state *HandoffState) *HandoffResult {
	logrus.WithFields(logrus.Fields{
		"from_agent": state.FromAgent,
		"to_agent":   state.ToAgent,
		"chat_id":    state.ChatID,
		"session_id": state.SessionID,
		"project":    state.ProjectPath,
	}).Info("Executing agent handoff")

	// Validate handoff
	if err := hm.validateHandoff(state); err != nil {
		return &HandoffResult{
			Success:   false,
			FromAgent: state.FromAgent,
			ToAgent:   state.ToAgent,
			Error:     err.Error(),
		}
	}

	if state.FromAgent == state.ToAgent {
		return &HandoffResult{
			Success:   true,
			FromAgent: state.FromAgent,
			ToAgent:   state.ToAgent,
			Message:   "",
			NextHint:  "",
		}
	}

	// Determine success message
	var message, nextHint string
	switch state.ToAgent {
	case AgentTypeClaudeCode:
		message = HandoffToCCPrompt()
		nextHint = "You can now use all Claude Code features. Type '@tb' to return to Smart Guide."
	case AgentTypeTinglyBox:
		message = HandoffToTBPrompt()
		nextHint = "I'm here to help with setup. Type '@cc' when ready to code."
	default:
		message = fmt.Sprintf("Switched to %s", state.ToAgent)
		nextHint = "Continue your conversation."
	}

	return &HandoffResult{
		Success:   true,
		FromAgent: state.FromAgent,
		ToAgent:   state.ToAgent,
		Message:   message,
		NextHint:  nextHint,
	}
}

// validateHandoff validates that a handoff can proceed
func (hm *HandoffManager) validateHandoff(state *HandoffState) error {
	// Check agent types
	if !validAgents[state.FromAgent] {
		return fmt.Errorf("invalid from_agent: %s", state.FromAgent)
	}

	if !validAgents[state.ToAgent] {
		return fmt.Errorf("invalid to_agent: %s", state.ToAgent)
	}

	// Check required fields
	if state.ChatID == "" {
		return fmt.Errorf("chat_id is required")
	}

	return nil
}

// SerializeState serializes handoff state to JSON
func SerializeState(state *HandoffState) ([]byte, error) {
	return json.Marshal(state)
}

// DeserializeState deserializes handoff state from JSON
func DeserializeState(data []byte) (*HandoffState, error) {
	var state HandoffState
	err := json.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize handoff state: %w", err)
	}
	return &state, nil
}

// DetectHandoffCommand detects if text is a handoff command.
// Returns the target agent type, whether it's a handoff, and any remaining text after the command.
// Examples:
//   - "@cc" -> (AgentTypeClaudeCode, true, "")
//   - "@cc help me" -> (AgentTypeClaudeCode, true, "help me")
//   - "hello" -> ("", false, "")
func DetectHandoffCommand(text string) (agentboot.AgentType, bool, string) {
	// Trim leading/trailing whitespace
	trimmed := strings.TrimSpace(text)

	// Check for handoff commands with possible trailing text
	if strings.HasPrefix(trimmed, "@cc ") || strings.HasPrefix(trimmed, "/cc ") {
		remaining := strings.TrimSpace(trimmed[4:])
		return AgentTypeClaudeCode, true, remaining
	}
	if strings.HasPrefix(trimmed, "@tb ") || strings.HasPrefix(trimmed, "/tb ") {
		remaining := strings.TrimSpace(trimmed[4:])
		return AgentTypeTinglyBox, true, remaining
	}

	// Check for exact match commands (no trailing text)
	switch trimmed {
	case "@cc", "/cc", "handoff", "switch to cc", "switch to claude", "cc":
		return AgentTypeClaudeCode, true, ""
	case "@tb", "/tb", "guide", "switch to tb", "switch to guide", "tb":
		return AgentTypeTinglyBox, true, ""
	}

	return "", false, ""
}

// GetAgentTypeString returns the string representation of an agent type
func GetAgentTypeString(agentType agentboot.AgentType) string {
	switch agentType {
	case AgentTypeTinglyBox:
		return "Smart Guide (@tb)"
	case AgentTypeClaudeCode:
		return "Claude Code (@cc)"
	case AgentTypeMock:
		return "Mock Agent"
	default:
		return string(agentType)
	}
}
