// Package remoteagent implements the "remote_agent" bot purpose: controlling
// Claude Code (@cc) and the SmartGuide agent (@tb) from a chat. It plugs into
// the bot host (internal/remote_control/bot) as a Consumer; the host owns the
// lifecycle, the shared channel prompter, and prompt-reply routing, while this
// package owns everything agent-specific — the inbound BotHandler, slash
// commands, agent routing/executors, and the streaming chat renderer.
package remoteagent

import (
	"github.com/tingly-dev/tingly-box/agentboot"

	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

// Agent routing constants
const (
	agentTinglyBox  agentboot.AgentType = "tingly-box" // @tb - Smart Guide (default)
	agentClaudeCode agentboot.AgentType = agentboot.AgentTypeClaude
	agentMock       agentboot.AgentType = "mock"
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// ResponseMeta contains metadata for response formatting
type ResponseMeta struct {
	ProjectPath string
	ChatID      string
	UserID      string
	SessionID   string
	AgentType   string // Current agent identifier (e.g., "tingly-box", "claude")
}

// getProjectPathForGroup retrieves the project path bound to a group chat.
func getProjectPathForGroup(chatStore bot.ChatStoreInterface, chatID string, platform string) (string, bool) {
	if chatStore == nil {
		return "", false
	}
	path, ok, err := chatStore.GetProjectPath(chatID)
	if err != nil {
		return "", false
	}
	return path, ok
}
