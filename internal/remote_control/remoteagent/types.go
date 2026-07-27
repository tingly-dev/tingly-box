// Package remoteagent implements the "remote_agent" bot purpose: controlling
// Claude Code (@cc) and the SmartGuide agent (@tb) from a chat. It plugs into
// the bot host (internal/remote_control/bot) as a Consumer; the host owns the
// lifecycle, the shared channel prompter, and prompt-reply routing, while this
// package owns everything agent-specific — the inbound BotHandler, slash
// commands, agent routing/executors, and the streaming chat renderer.
package remoteagent

import (
	"github.com/tingly-dev/tingly-box/agentboot"

	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

// Agent routing constants. The identity strings are owned elsewhere —
// smart_guide names the handoff targets, agentboot names its agents — these
// are just the agentboot-typed views this package routes on.
const (
	agentTinglyBox                      = agentboot.AgentType(smart_guide.AgentTypeTinglyBox) // @tb - Smart Guide (default)
	agentClaudeCode                     = agentboot.AgentTypeClaude
	agentMock       agentboot.AgentType = "mock"
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// ResponseMeta carries the two values response footers render: the acting
// agent and the chat's project path. It is shared by pointer between the
// router, the executors, and the SmartGuide completion callback so a
// mid-execution project change is reflected in the closing footer.
type ResponseMeta struct {
	ProjectPath string
	AgentType   string // Current agent identifier (e.g., "tingly-box", "claude")
}
