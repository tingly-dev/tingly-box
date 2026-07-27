package remoteagent

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

	return router
}

// RegisterExecutor registers an agent executor
func (r *AgentRouter) RegisterExecutor(executor AgentExecutor) {
	r.executors[executor.GetAgentType()] = executor
	logrus.WithField("agentType", executor.GetAgentType()).Debug("Registered agent executor")
}

// Execute routes the execution request to the appropriate agent executor.
// It resolves project path, session, and builds shared *ResponseMeta before delegating.
func (r *AgentRouter) Execute(ctx context.Context, agentType agentboot.AgentType, req ExecutionRequest) error {
	executor, exists := r.executors[agentType]
	if !exists {
		return fmt.Errorf("no executor found for agent type: %s", agentType)
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
		AgentType:   string(agentType),
	}

	// 4. Setup cancellable context + /stop bookkeeping via the execution
	// registry (fails with errExecutionBusy when this chat is already running).
	execCtx, cancel := context.WithCancel(ctx)
	if err := r.deps.Executions.begin(req.HCtx.ChatID, cancel); err != nil {
		cancel()
		return err
	}

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

	// 6. Delegate to executor
	err := executor.Execute(execCtx, prepared)

	// Always cleanup on return
	r.deps.Executions.end(req.HCtx.ChatID)
	cancel()

	return err
}
