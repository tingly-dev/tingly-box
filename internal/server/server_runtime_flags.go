package server

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) initGuardrailsRuntime() {
	runtime := s.currentGuardrailsRuntime()
	if (runtime != nil && runtime.PolicyEngine() != nil) || s.config == nil {
		return
	}

	if !s.guardrailsEnabled() {
		return
	}

	cfgPath, err := FindGuardrailsConfig(s.config.ConfigDir)
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

	s.setGuardrailsRuntime(&guardrails.Guardrails{Policy: policy}, "guardrails init")
	logrus.Infof("Guardrails enabled with config: %s", cfgPath)
}

func (s *Server) ensureDefaultGuardrailsConfig() (string, error) {
	if s == nil || s.config == nil || s.config.ConfigDir == "" {
		return "", fmt.Errorf("config directory not set")
	}

	path := GetGuardrailsConfigPath(s.config.ConfigDir)
	cfg := guardrailscore.Config{
		Groups: []guardrailscore.PolicyGroup{
			{
				ID:      guardrailscore.DefaultPolicyGroupID,
				Name:    "Default",
				Enabled: true,
			},
		},
	}

	data, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}

	logrus.Infof("Created default guardrails config: %s", path)
	return path, nil
}

func (s *Server) guardrailsEnabled() bool {
	if s.config == nil {
		return false
	}
	return s.config.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionGuardrails) ||
		s.config.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionGuardrails)
}

// mcpEnabled checks if MCP feature is enabled via scenario flag
func (s *Server) mcpEnabled() bool {
	if s.config == nil {
		return false
	}
	return s.config.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionMCP) ||
		s.config.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionMCP)
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
	if s.config.GetToolConfig(db.ToolTypeMCPRuntime, &mcpCfg) {
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
		s.setGuardrailsRuntime(&guardrails.Guardrails{}, "guardrails disable")
		logrus.Debug("Guardrails disabled via config")
		return
	}

	if s.currentGuardrailsRuntime() == nil {
		s.initGuardrailsRuntime()
	}
}
