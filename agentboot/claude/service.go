package claude

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	claudesession "github.com/tingly-dev/tingly-box/agentboot/claude/session"
)

// NewService composes the provider-neutral AgentService with the Claude Code
// agent and Claude's on-disk historical session reader.
//
// An empty Config.ClaudeProjectsDir uses Claude Code's canonical
// ~/.claude/projects directory.
func NewService(config agentboot.Config) (*agentboot.AgentService, error) {
	reader, err := claudesession.NewStore(config.ClaudeProjectsDir)
	if err != nil {
		return nil, fmt.Errorf("initialize Claude session reader: %w", err)
	}

	service, err := agentboot.NewAgentService(config, agentboot.WithSessionReader(reader))
	if err != nil {
		return nil, err
	}
	service.RegisterAgent(agentboot.AgentTypeClaude, NewAgent(config))
	return service, nil
}
