package server

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	"github.com/tingly-dev/tingly-box/internal/protocolserver"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) guardrailsEnabled() bool {
	if s.config == nil {
		return false
	}
	return s.config.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionGuardrails) ||
		s.config.GetScenarioFlag(typ.ScenarioClaudeCode, constant.ExtensionGuardrails)
}

// mcpEnabled checks if MCP feature is enabled via scenario flag
func (s *Server) mcpEnabled() bool {
	return protocolserver.MCPEnabled(s.config)
}

// projectsEnabled reports whether Projects (a coordinator conversation
// running Claude Code threads in git worktrees, see .design/projects.md) is
// turned on. Off by default: it runs an arbitrary local coding agent on the
// host, so it opts in the same way guardrails/MCP do rather than being
// always-on.
func (s *Server) projectsEnabled() bool {
	if s.config == nil {
		return false
	}
	return s.config.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionProjects)
}

// adoptProjectsForRemote turns Projects on, once, for an install that was
// already chatting through Remote: since Projects' switch also gates chat
// control (.design/remote-into-projects.md), leaving it off would cut those
// chats off on upgrade. A switch that was ever set, either way, is left
// alone, so this decides only for installs that never chose.
func (s *Server) adoptProjectsForRemote() {
	if s.config == nil || s.config.HasScenarioFlag(typ.ScenarioGlobal, constant.ExtensionProjects) {
		return
	}
	sm := s.config.StoreManager()
	if sm == nil || sm.ImBotSettings() == nil {
		return
	}
	bots, err := sm.ImBotSettings().ListEnabledSettings()
	if err != nil || len(bots) == 0 {
		return
	}
	if err := s.config.SetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionProjects, true); err != nil {
		logrus.WithError(err).Warn("Failed to turn on Projects for the IM bots in use")
		return
	}
	logrus.Infof("Turned on Projects: %d IM bot(s) in use now chat through it", len(bots))
}

func (s *Server) initGuardrailsRuntime() {
	runtime := s.guardrailsState.Current()
	if (runtime != nil && runtime.PolicyEngine() != nil) || s.config == nil {
		return
	}

	if !s.guardrailsEnabled() {
		return
	}

	cfgPath, err := config.FindConfig(s.config.ConfigDir)
	if err != nil {
		if !strings.Contains(err.Error(), "no guardrails config") {
			logrus.WithError(err).Warn("Failed to locate guardrails config")
			return
		}
		cfgPath, err = s.ensureDefaultGuardrailsConfig()
		if err != nil {
			logrus.WithError(err).Warn("Failed to create default guardrails config")
			return
		}
	}

	cfg, err := guardrails.LoadConfig(cfgPath)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load guardrails config")
		return
	}

	policy, err := guardrailsevaluate.BuildPolicyEngine(cfg, guardrailsevaluate.Dependencies{})
	if err != nil {
		logrus.WithError(err).Warn("Failed to build guardrails policy engine")
		return
	}

	s.guardrailsState.Set(&guardrails.Guardrails{Policy: policy}, "guardrails init")
	logrus.Infof("Guardrails enabled with config: %s", cfgPath)
}

func (s *Server) ensureDefaultGuardrailsConfig() (string, error) {
	if s == nil || s.config == nil || s.config.ConfigDir == "" {
		return "", fmt.Errorf("config directory not set")
	}

	path := config.ConfigPath(s.config.ConfigDir)
	cfg := guardrailscore.Config{
		Groups: []guardrailscore.PolicyGroup{
			{
				ID:      guardrailscore.DefaultPolicyGroupID,
				Name:    "Default",
				Enabled: true,
			},
		},
	}

	data, err := config.MarshalConfig(cfg)
	if err != nil {
		return "", err
	}
	if err := config.WriteFileAtomic(path, data); err != nil {
		return "", err
	}

	logrus.Infof("Created default guardrails config: %s", path)
	return path, nil
}

// mcpStripDisabledToolsEnabled returns whether dangerous disabled MCP strip is enabled.
func (s *Server) mcpStripDisabledToolsEnabled() bool {
	if s.config == nil {
		return false
	}
	cfg := s.config.GetMCPRuntimeConfig()
	if cfg == nil {
		return false
	}
	return cfg.StripDisabledMCPTools
}

// mcpMode returns the current MCP runtime mode
func (s *Server) mcpMode() typ.MCPMode {
	if s.config == nil {
		return ""
	}
	var mcpCfg typ.MCPRuntimeConfig
	if s.config.GetToolConfig(config.ToolTypeMCPRuntime, &mcpCfg) {
		if mcpCfg.Mode == "" {
			return typ.MCPModeClienttool // default mode
		}
		return mcpCfg.Mode
	}
	return typ.MCPModeClienttool // default mode
}

func (s *Server) syncGuardrailsFromConfig() {
	if s.config == nil {
		return
	}

	if !s.guardrailsEnabled() {
		s.guardrailsState.Set(&guardrails.Guardrails{}, "guardrails disable")
		logrus.Debug("Guardrails disabled via config")
		return
	}

	if s.guardrailsState.Current() == nil {
		s.initGuardrailsRuntime()
	}
}
