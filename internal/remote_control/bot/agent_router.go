package bot

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// AgentRouter routes execution requests to the appropriate agent executor
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

	// Register default executors
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

// Execute routes the execution request to the appropriate agent executor
func (r *AgentRouter) Execute(ctx context.Context, agentType agentboot.AgentType, req ExecutionRequest) (*ExecutionResult, error) {
	executor, exists := r.executors[agentType]
	if !exists {
		return nil, fmt.Errorf("no executor found for agent type: %s", agentType)
	}

	logrus.WithFields(logrus.Fields{
		"agentType": agentType,
		"chatID":    req.HCtx.ChatID,
		"textLen":   len(req.Text),
	}).Info("Routing request to agent executor")

	return executor.Execute(ctx, req)
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
