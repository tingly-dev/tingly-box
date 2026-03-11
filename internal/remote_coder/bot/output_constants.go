package bot

import (
	"fmt"
	"strings"
)

// Output format constants for bot messages
// Centralized for easy customization and i18n support
const (
	// Icons
	IconProject = "📁"  // Project/folder
	IconChat    = "💬"  // Chat/conversation
	IconUser    = "👤"  // User
	IconSession = "🔄"  // Session
	IconAgentTB = "🎯"  // Tingly-Box agent (@tb)
	IconAgentCC = "💬"  // Claude Code agent (@cc)
	IconDone    = "✅"  // Task completed
	IconError   = "❌"  // Error
	IconWarning = "⚠️" // Warning
	IconStop    = "🛑"  // Stopped
	IconProcess = "⏳"  // Processing
	IconMock    = "🧪"  // Mock agent
)

// Agent display names
const (
	AgentNameTB        = "@tb" // Tingly-Box short name
	AgentNameCC        = "@cc" // Claude Code short name
	AgentNameTinglyBox = "tingly-box"
	AgentNameClaude    = "claude"
)

// Separator line for message formatting
const (
	SeparatorLine = "───────────────"
	SeparatorFull = "━━━━━━━━━━━━━━━━━━━━"
)

// Status messages
const (
	MsgProcessing     = "Processing..."
	MsgTaskDone       = "Task done"
	MsgTaskStopped    = "Task stopped"
	MsgContinueOrHelp = "Continue or /help."
	MsgNoRunningTask  = "No running task to stop."
)

// Format templates (use with fmt.Sprintf)
const (
	// Status line formats
	FormatProjectLine = "%s %s\n" // icon + path
	FormatAgentLine   = "%s %s\n" // icon + agent name
	FormatDebugLine   = "%s %s\n" // icon + id value

	// Completion message formats
	FormatDoneWithCtx  = "%s %s done | %s %s\n%s" // icon + agent + path_icon + path + continue_msg
	FormatDoneSimple   = "%s %s done\n%s"         // icon + agent + continue_msg
	FormatDoneWithProj = "%s %s done | %s %s"     // icon + agent + path_icon + path (single line)
)

// OutputBehavior controls what is shown in bot messages
type OutputBehavior struct {
	Debug   bool // Show message IDs (chat_id, user_id, session_id)
	Verbose bool // Send intermediate processing messages
}

// DefaultOutputBehavior returns the default output behavior
func DefaultOutputBehavior() OutputBehavior {
	return OutputBehavior{
		Debug:   false,
		Verbose: true,
	}
}

// GetOutputBehavior extracts output behavior from bot setting
func (s BotSetting) GetOutputBehavior() OutputBehavior {
	verbose := true // default
	if s.Verbose != nil {
		verbose = *s.Verbose
	}
	return OutputBehavior{
		Debug:   s.Debug,
		Verbose: verbose,
	}
}

// GetAgentIcon returns the icon for an agent type
func GetAgentIcon(agentType string) string {
	switch agentType {
	case AgentNameTinglyBox, AgentNameTB:
		return IconAgentTB
	case AgentNameClaude, AgentNameCC, "claude-code":
		return IconAgentCC
	default:
		return IconAgentCC
	}
}

// GetAgentDisplayName returns the short display name for an agent type
func GetAgentDisplayName(agentType string) string {
	switch agentType {
	case AgentNameTinglyBox:
		return AgentNameTB
	case AgentNameClaude, "claude-code":
		return AgentNameCC
	default:
		return agentType
	}
}

// ShortenPath creates a readable short version of a path
// e.g., "/Users/yz/Project/101-project/tingly-box" -> "101-project/tingly-box"
func ShortenPath(path string) string {
	if path == "" {
		return ""
	}

	// Normalize path separators
	parts := splitPath(path)

	// Already short enough
	if len(parts) <= 2 {
		return joinPath(parts)
	}

	// Show last 2 parts
	return joinPath(parts[len(parts)-2:])
}

// ShortenID truncates an ID to a readable length
// e.g., "a1b2c3d4e5f6g7h8" -> "a1b2c3d4"
func ShortenID(id string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 8
	}
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// BuildDoneMessage creates a completion message with optional context
func BuildDoneMessage(agentType, projectPath string, behavior OutputBehavior) string {
	icon := GetAgentIcon(agentType)
	agentName := GetAgentDisplayName(agentType)

	if projectPath != "" {
		shortPath := ShortenPath(projectPath)
		return fmt.Sprintf(FormatDoneWithProj, IconDone, icon+" "+agentName, IconProject, shortPath)
	}
	return fmt.Sprintf("%s %s %s", IconDone, agentName, MsgTaskDone)
}

// Helper functions for path handling (avoiding filepath import issues)
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	// Handle both / and \ separators
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	// Filter empty parts
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func joinPath(parts []string) string {
	return strings.Join(parts, "/")
}
