package runtime

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RegisterBuiltinTools ensures built-in tools are registered in the MCP configuration.
// If webtools already exists, it updates the command path to use the current binary's absolute path.
func RegisterBuiltinTools(getConfig func() *typ.MCPRuntimeConfig, setConfig func(toolType string, config interface{}) error) error {
	cfg := getConfig()
	if cfg == nil {
		cfg = &typ.MCPRuntimeConfig{}
	}

	// Check if builtins already exist
	var existingWebtools *typ.MCPSourceConfig
	var existingAdvisor *typ.MCPSourceConfig
	webtoolsIndex := -1
	advisorIndex := -1
	for i, source := range cfg.Sources {
		if source.ID == mcptools.BuiltinWebtoolsSourceID {
			existingWebtools = &cfg.Sources[i]
			webtoolsIndex = i
		}
		if source.ID == mcptools.BuiltinAdvisorSourceID {
			existingAdvisor = &cfg.Sources[i]
			advisorIndex = i
		}
	}

	// Get the absolute path of the current executable
	currentExecPath, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Error("mcp: failed to get current executable path")
		return err
	}
	currentExecPath, err = filepath.Abs(currentExecPath)
	if err != nil {
		logrus.WithError(err).Warn("mcp: failed to resolve absolute path, using original")
	}

	// Preserve user's environment variables (especially SERPER_API_KEY)
	preservedEnv := make(map[string]string)
	if existingWebtools != nil && existingWebtools.Env != nil {
		for k, v := range existingWebtools.Env {
			preservedEnv[k] = v
		}
	}

	// Ensure SERPER_API_KEY is always present (use ${SERPER_API_KEY} to reference system env)
	if _, exists := preservedEnv["SERPER_API_KEY"]; !exists {
		preservedEnv["SERPER_API_KEY"] = "${SERPER_API_KEY}"
	}

	// Create webtools configuration
	visibility := typ.ToolVisibilityClient
	enabled := typ.BoolPtr(false)
	tools := mcptools.DefaultBuiltinWebtoolNames()
	if existingWebtools != nil {
		if existingWebtools.Enabled != nil {
			enabled = existingWebtools.Enabled
		}
		if existingWebtools.Visibility != "" {
			visibility = existingWebtools.Visibility
		}
		if len(existingWebtools.Tools) > 0 {
			tools = existingWebtools.Tools
		}
	}
	builtinWebtools := typ.MCPSourceConfig{
		ID:         mcptools.BuiltinWebtoolsSourceID,
		Name:       mcptools.BuiltinWebtoolsSourceName,
		Transport:  "stdio",
		Command:    currentExecPath,         // Use absolute path of current binary
		Args:       []string{"mcp-builtin"}, // Subcommand to start builtin server
		Enabled:    enabled,
		Visibility: visibility,
		Tools:      tools,
		Env:        preservedEnv, // Preserve user's environment variables
	}

	// Update or append webtools configuration
	if webtoolsIndex >= 0 {
		cfg.Sources[webtoolsIndex] = builtinWebtools
		logrus.Info("mcp: updated builtin webtools configuration")
	} else {
		cfg.Sources = append(cfg.Sources, builtinWebtools)
		logrus.Info("mcp: registered builtin webtools configuration")
	}

	// Create advisor configuration and register as virtual tool.
	advisorEnabled := typ.BoolPtr(false) // default: disabled
	advisorVisibility := typ.ToolVisibilityServer
	advisorTools := mcptools.DefaultBuiltinAdvisorToolNames()
	advisorEnv := map[string]string{}
	advisorCfg := &typ.AdvisorConfig{
		MaxUsesPerRequest: 3,
		MaxTokens:         4096,
	}
	if existingAdvisor != nil {
		if existingAdvisor.Enabled != nil {
			advisorEnabled = existingAdvisor.Enabled
		}
		if existingAdvisor.Visibility != "" {
			advisorVisibility = existingAdvisor.Visibility
		}
		if len(existingAdvisor.Tools) > 0 {
			advisorTools = existingAdvisor.Tools
		}
		// Preserve user's custom environment variables
		for k, v := range existingAdvisor.Env {
			advisorEnv[k] = v
		}
		if existingAdvisor.Advisor != nil {
			copied := *existingAdvisor.Advisor
			advisorCfg = &copied
		}
	}
	builtinAdvisor := typ.MCPSourceConfig{
		ID:         mcptools.BuiltinAdvisorSourceID,
		Name:       mcptools.BuiltinAdvisorSourceName,
		Enabled:    advisorEnabled,
		Visibility: advisorVisibility,
		Tools:      advisorTools,
		Env:        advisorEnv,
		Advisor:    advisorCfg,
	}

	// Update or append advisor configuration
	if advisorIndex >= 0 {
		cfg.Sources[advisorIndex] = builtinAdvisor
		logrus.Info("mcp: updated builtin advisor configuration")
	} else {
		cfg.Sources = append(cfg.Sources, builtinAdvisor)
		logrus.Info("mcp: registered builtin advisor configuration")
	}

	// Note: We no longer create a transport-based MCPSourceConfig for adviser.
	// Instead, the virtual tool is registered directly into the runtime's VirtualRegistry.
	// This is done in Runtime initialization, not here.

	if err := setConfig("mcp_runtime", cfg); err != nil {
		logrus.WithError(err).Error("mcp: failed to save builtin webtools configuration")
		return err
	}

	return nil
}
