package claude

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot"
	claudesession "github.com/tingly-dev/tingly-box/agentboot/claude/session"
)

type serviceConfig struct {
	projectsDir string
}

// ServiceOption configures Claude-owned service composition.
type ServiceOption func(*serviceConfig)

// WithProjectsDir selects the Claude Code session-history directory.
// An empty value uses ~/.claude/projects.
func WithProjectsDir(path string) ServiceOption {
	return func(config *serviceConfig) {
		config.projectsDir = path
	}
}

// NewService composes the provider-neutral AgentService with the Claude Code
// agent and Claude's on-disk historical session reader.
//
// Without WithProjectsDir it uses Claude Code's canonical ~/.claude/projects
// directory.
func NewService(config agentboot.Config, options ...ServiceOption) (*agentboot.AgentService, error) {
	serviceConfig := serviceConfig{}
	for _, option := range options {
		if option != nil {
			option(&serviceConfig)
		}
	}

	reader, err := claudesession.NewStore(serviceConfig.projectsDir)
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
