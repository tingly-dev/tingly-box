package agentboot

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// AgentService is the primary entry point for external callers that need to:
//   - Query projects and sessions associated with an agent
//   - Execute a prompt against a new or existing session
//
// It composes an [AgentBoot] registry with a [common.SessionStore] for queries
// and an optional [session.Store] for runtime lifecycle notifications.
// The underlying Runner/Driver/Transport/ExecutionHandle pipeline is unchanged.
type AgentService struct {
	boot *AgentBoot
}

// NewAgentService creates an AgentService from the given config.
// Agent implementations must be registered via RegisterAgent before calling Execute.
func NewAgentService(config Config) (*AgentService, error) {
	boot, err := New(config)
	if err != nil {
		return nil, err
	}
	return &AgentService{boot: boot}, nil
}

// RegisterAgent registers an agent implementation for the given type.
func (s *AgentService) RegisterAgent(agentType AgentType, agent Agent) {
	s.boot.RegisterAgent(agentType, agent)
}

// Boot returns the underlying AgentBoot for callers that need low-level access.
func (s *AgentService) Boot() *AgentBoot {
	return s.boot
}

// --- Query API ---

// ListProjects returns all project paths that have at least one recorded session.
func (s *AgentService) ListProjects(ctx context.Context) ([]string, error) {
	return s.boot.ListProjects(ctx)
}

// ListSessions returns up to limit sessions for the given project, newest first.
// Pass limit <= 0 to return all sessions.
func (s *AgentService) ListSessions(ctx context.Context, projectPath string, limit int) ([]common.SessionMetadata, error) {
	if limit <= 0 {
		return s.boot.store.ListSessions(ctx, projectPath)
	}
	return s.boot.ListRecentSessions(ctx, projectPath, limit)
}

// GetSession returns metadata for a specific session by ID.
func (s *AgentService) GetSession(ctx context.Context, sessionID string) (*common.SessionMetadata, error) {
	if s.boot.store == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	return s.boot.store.GetSession(ctx, sessionID)
}

// GetSessionSummary returns head and tail events of a session.
func (s *AgentService) GetSessionSummary(ctx context.Context, sessionID string, firstN, lastM int) (*common.SessionSummary, error) {
	return s.boot.GetSessionSummary(ctx, sessionID, firstN, lastM)
}

// --- Execution API ---

// Execute runs a prompt against the specified agent type and project path.
// A new session is started unless opts.SessionID is set (in which case that
// session is reused but not resumed — use ExecuteSession to resume).
//
// opts.Store, if nil, is left unset (no lifecycle callbacks).
// opts.ProjectPath is overridden by the projectPath argument.
func (s *AgentService) Execute(ctx context.Context, agentType AgentType, projectPath string, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	agent, err := s.boot.GetAgent(agentType)
	if err != nil {
		return nil, fmt.Errorf("agentservice: %w", err)
	}

	opts.ProjectPath = projectPath
	return agent.Execute(ctx, prompt, opts)
}

// ExecuteSession resumes an existing session by ID.
// It looks up the session's project path from the store, then executes with
// Resume=true so the agent continues the conversation.
func (s *AgentService) ExecuteSession(ctx context.Context, sessionID string, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	meta, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("agentservice: session %q not found: %w", sessionID, err)
	}

	agent, err := s.boot.GetDefaultAgent()
	if err != nil {
		return nil, fmt.Errorf("agentservice: %w", err)
	}

	opts.ProjectPath = meta.ProjectPath
	opts.SessionID = sessionID
	opts.Resume = true
	return agent.Execute(ctx, prompt, opts)
}

// ExecuteSessionWithAgent is like ExecuteSession but uses a specific agent type.
func (s *AgentService) ExecuteSessionWithAgent(ctx context.Context, agentType AgentType, sessionID string, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	meta, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("agentservice: session %q not found: %w", sessionID, err)
	}

	agent, err := s.boot.GetAgent(agentType)
	if err != nil {
		return nil, fmt.Errorf("agentservice: %w", err)
	}

	opts.ProjectPath = meta.ProjectPath
	opts.SessionID = sessionID
	opts.Resume = true
	return agent.Execute(ctx, prompt, opts)
}
