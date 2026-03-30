package bot

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// AgentRouter routes execution requests to the appropriate agent executor.
// It resolves common concerns (project path, session, meta, cancel context) once,
// then delegates to the specific executor.
type AgentRouter struct {
	executors map[agentboot.AgentType]AgentExecutor
	deps      *ExecutorDependencies
}

// NewAgentRouter creates a new agent router with the given dependencies
func NewAgentRouter(deps *ExecutorDependencies) *AgentRouter {
	router := &AgentRouter{
		executors: make(map[agentboot.AgentType]AgentExecutor),
		deps:      deps,
	}

	router.RegisterExecutor(NewClaudeCodeExecutor(deps))
	router.RegisterExecutor(NewSmartGuideExecutor(deps))
	router.RegisterExecutor(NewMockAgentExecutor(deps))

	return router
}

// RegisterExecutor registers an agent executor
func (r *AgentRouter) RegisterExecutor(executor AgentExecutor) {
	r.executors[executor.GetAgentType()] = executor
	logrus.WithField("agentType", executor.GetAgentType()).Debug("Registered agent executor")
}

// Execute routes the execution request to the appropriate agent executor.
// It resolves project path, session, and builds shared *ResponseMeta before delegating.
func (r *AgentRouter) Execute(ctx context.Context, agentType agentboot.AgentType, req ExecutionRequest) (*ExecutionResult, error) {
	executor, exists := r.executors[agentType]
	if !exists {
		return nil, fmt.Errorf("no executor found for agent type: %s", agentType)
	}

	// 1. Resolve project path
	projectPath := r.deps.resolveProjectPath(req.HCtx.ChatID, req.ProjectPath)

	// 2. Resolve session (session-based agents only; SmartGuide uses chatID)
	var sessionID string
	var isNewSession bool
	var permissionMode string
	if agentType != agentTinglyBox {
		sessionID, isNewSession, permissionMode = r.deps.resolveSession(req.HCtx.ChatID, string(agentType), projectPath)
	} else {
		sessionID = req.HCtx.ChatID
	}

	// 3. Build shared meta
	meta := &ResponseMeta{
		ProjectPath: projectPath,
		AgentType:   agentTypeToName(agentType),
		SessionID:   sessionID,
		ChatID:      req.HCtx.ChatID,
		UserID:      req.HCtx.SenderID,
	}

	// 4. Setup cancellable context + /stop bookkeeping
	execCtx, cancel := context.WithCancel(ctx)
	r.deps.RunningCancelMu.Lock()
	r.deps.RunningCancel[req.HCtx.ChatID] = cancel
	r.deps.RunningCancelMu.Unlock()

	// 5. Build prepared request
	prepared := PreparedRequest{
		HCtx:           req.HCtx,
		Text:           req.Text,
		ProjectPath:    projectPath,
		Meta:           meta,
		SessionID:      sessionID,
		IsNewSession:   isNewSession,
		PermissionMode: permissionMode,
		ReplyTo:        req.ReplyToMessageID,
	}

	logrus.WithFields(logrus.Fields{
		"agentType":   agentType,
		"chatID":      req.HCtx.ChatID,
		"sessionID":   sessionID,
		"projectPath": projectPath,
		"newSession":  isNewSession,
	}).Info("Routing prepared request to executor")

	// 6. Delegate to executor (it is responsible for calling defer cleanup)
	result, err := executor.Execute(execCtx, prepared)

	// Always cleanup cancel on return
	r.deps.RunningCancelMu.Lock()
	delete(r.deps.RunningCancel, req.HCtx.ChatID)
	r.deps.RunningCancelMu.Unlock()
	cancel()

	return result, err
}

// GetExecutor returns the executor for a given agent type
func (r *AgentRouter) GetExecutor(agentType agentboot.AgentType) (AgentExecutor, bool) {
	executor, exists := r.executors[agentType]
	return executor, exists
}

// ListExecutors returns all registered agent types
func (r *AgentRouter) ListExecutors() []agentboot.AgentType {
	types := make([]agentboot.AgentType, 0, len(r.executors))
	for agentType := range r.executors {
		types = append(types, agentType)
	}
	return types
}
