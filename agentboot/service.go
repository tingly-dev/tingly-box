package agentboot

import (
	"context"
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/agentboot/history"
)

// errSessionReaderNotConfigured is returned when history APIs are used without
// an injected provider-specific reader.
var errSessionReaderNotConfigured = fmt.Errorf("agentservice: session reader not configured")

// AgentService is the single entry point for callers that need to:
//   - Query projects and sessions associated with an agent
//   - Execute a prompt against a new or existing session, either as a raw
//     [ExecutionHandle] (Execute*) or driven to completion (Run)
//
// It owns the agent registry and the optional session reader. The underlying
// Runner/Driver/Transport/ExecutionHandle pipeline is unchanged.
type AgentService struct {
	mu            sync.RWMutex
	config        Config
	agents        map[AgentType]Agent
	sessionReader history.SessionReader
}

// ServiceOption configures an [AgentService] integration.
type ServiceOption func(*AgentService) error

// WithSessionReader injects read-only historical session access.
func WithSessionReader(reader history.SessionReader) ServiceOption {
	return func(service *AgentService) error {
		if reader == nil {
			return fmt.Errorf("agentservice: session reader is nil")
		}
		service.sessionReader = reader
		return nil
	}
}

// NewAgentService creates a provider-neutral AgentService from the given
// config. Agent implementations must be registered via RegisterAgent before
// executing. Provider history is optional and can be injected with
// [WithSessionReader].
func NewAgentService(config Config, options ...ServiceOption) (*AgentService, error) {
	if config.DefaultAgent == "" {
		config.DefaultAgent = AgentTypeClaude
	}
	if config.DefaultFormat == "" {
		config.DefaultFormat = OutputFormatStreamJSON
	}
	if config.StreamBufferSize == 0 {
		config.StreamBufferSize = 100
	}

	service := &AgentService{
		config: config,
		agents: make(map[AgentType]Agent),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	return service, nil
}

// RegisterAgent registers an agent implementation for the given type.
func (s *AgentService) RegisterAgent(agentType AgentType, agent Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agentType] = agent
}

// SetDefaultAgent selects the registered agent used when execution APIs receive
// an empty AgentType.
func (s *AgentService) SetDefaultAgent(agentType AgentType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.agents[agentType]; !exists {
		return fmt.Errorf("agent type not registered: %s", agentType)
	}
	s.config.DefaultAgent = agentType
	return nil
}

// RegisteredAgents returns the currently registered agent types.
func (s *AgentService) RegisteredAgents() []AgentType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	types := make([]AgentType, 0, len(s.agents))
	for agentType := range s.agents {
		types = append(types, agentType)
	}
	return types
}

// Config returns a snapshot of the service configuration.
func (s *AgentService) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// --- Query API ---

// ListProjects returns all project paths that have at least one recorded session.
func (s *AgentService) ListProjects(ctx context.Context) ([]string, error) {
	if s.sessionReader == nil {
		return nil, errSessionReaderNotConfigured
	}
	return s.sessionReader.ListProjects(ctx)
}

// ListSessions returns up to limit sessions for the given project, newest first.
// Pass limit <= 0 to return all sessions.
func (s *AgentService) ListSessions(ctx context.Context, projectPath string, limit int) ([]history.SessionMetadata, error) {
	if s.sessionReader == nil {
		return nil, errSessionReaderNotConfigured
	}
	if limit <= 0 {
		return s.sessionReader.ListSessions(ctx, projectPath)
	}
	return s.sessionReader.GetRecentSessions(ctx, projectPath, limit)
}

// GetSession returns metadata for a specific session by ID.
func (s *AgentService) GetSession(ctx context.Context, sessionID string) (*history.SessionMetadata, error) {
	if s.sessionReader == nil {
		return nil, errSessionReaderNotConfigured
	}
	return s.sessionReader.GetSession(ctx, sessionID)
}

// GetSessionSummary returns head and tail events of a session.
func (s *AgentService) GetSessionSummary(ctx context.Context, sessionID string, firstN, lastM int) (*history.SessionSummary, error) {
	if s.sessionReader == nil {
		return nil, errSessionReaderNotConfigured
	}
	return s.sessionReader.GetSessionSummary(ctx, sessionID, firstN, lastM)
}

// --- Execution API ---

// resolveAgent returns the agent for agentType, or the default agent when
// agentType is empty.
func (s *AgentService) resolveAgent(agentType AgentType) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if agentType == "" {
		agentType = s.config.DefaultAgent
	}
	agent, exists := s.agents[agentType]
	if !exists {
		return nil, fmt.Errorf("agent type not supported: %s", agentType)
	}
	return agent, nil
}

// Execute runs a prompt against the specified agent type and project path and
// returns a raw [ExecutionHandle] for callers that want event-level control.
// Pass an empty agentType to use the default agent.
//
// A new session is started unless opts.SessionID is set with opts.Resume.
func (s *AgentService) Execute(ctx context.Context, agentType AgentType, projectPath string, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	agent, err := s.resolveAgent(agentType)
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
	return s.ExecuteSessionWithAgent(ctx, "", sessionID, prompt, opts)
}

// ExecuteSessionWithAgent is like ExecuteSession but uses a specific agent type
// (empty agentType uses the default agent).
func (s *AgentService) ExecuteSessionWithAgent(ctx context.Context, agentType AgentType, sessionID string, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	meta, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("agentservice: session %q not found: %w", sessionID, err)
	}

	agent, err := s.resolveAgent(agentType)
	if err != nil {
		return nil, fmt.Errorf("agentservice: %w", err)
	}

	opts.ProjectPath = meta.ProjectPath
	opts.SessionID = sessionID
	opts.Resume = true
	return agent.Execute(ctx, prompt, opts)
}

// RunRequest bundles the inputs for a high-level [AgentService.Run].
type RunRequest struct {
	// AgentType selects the agent; empty uses the default agent.
	AgentType AgentType
	// ProjectPath is the working directory for the run.
	ProjectPath string
	// Prompt is the user message.
	Prompt string
	// Opts carries session id/resume, env, permission mode, lifecycle Store, etc.
	Opts ExecutionOptions
}

// Run executes the request and drives the resulting handle to completion via
// [RunWithPrompter]: it streams MessageEvent.Raw values to sink (nil to drop),
// routes Approval/Ask requests to prompter, and returns the aggregated Result.
//
// This is the high-level convenience entry point — callers that need
// event-level control should use [AgentService.Execute] and consume the handle
// directly.
func (s *AgentService) Run(ctx context.Context, req RunRequest, prompter Prompter, sink MessageSink) (*Result, error) {
	handle, err := s.Execute(ctx, req.AgentType, req.ProjectPath, req.Prompt, req.Opts)
	if err != nil {
		return nil, err
	}
	return RunWithPrompter(ctx, handle, prompter, sink)
}
