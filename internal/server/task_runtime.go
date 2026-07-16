package server

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/codex"
	taskapi "github.com/tingly-dev/tingly-box/internal/server/module/task"
	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

func (s *Server) initTaskRuntime() {
	if s == nil || s.config == nil || s.config.StoreManager() == nil {
		return
	}
	store := s.config.StoreManager().Tasks()
	if store == nil {
		return
	}

	claudeConfig := agentboot.DefaultConfig()
	claudeAgent := claude.NewAgent(claudeConfig)
	codexAgent := codex.NewAgent(codex.DefaultConfig())
	agents := map[agenttask.AgentKind]agentboot.Agent{
		agenttask.AgentClaude: claudeAgent,
		agenttask.AgentCodex:  codexAgent,
	}
	tbClient := tbclient.NewTBClient(s.config, s.config.StoreManager().Provider())
	envResolver := func(ctx context.Context, agent agenttask.AgentKind) ([]string, error) {
		if agent == agenttask.AgentClaude {
			return tbClient.GetClaudeCodeEnv(ctx)
		}
		// Codex routing is owned by its native CODEX_HOME/config.toml. The
		// existing Codex import/apply flow points that config at TB; injecting
		// a second per-process config here would split session ownership.
		return nil, nil
	}

	manager := coretask.NewManager(store)
	agentHandler := agenttask.NewHandler(agents, envResolver)
	if err := manager.Register(agentHandler); err != nil {
		logrus.WithError(err).Warn("Task runtime handler registration failed")
		return
	}
	if err := manager.Start(s.ctx); err != nil {
		logrus.WithError(err).Warn("Task runtime failed to start")
		return
	}

	s.taskManager = manager
	s.taskAPI = taskapi.NewHandler(manager, s.config.ConfigDir, agents)
	logrus.Debug("Task runtime initialized")
}
