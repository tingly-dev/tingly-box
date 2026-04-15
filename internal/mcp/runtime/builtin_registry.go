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

	// Check if webtools already exists
	var existingWebtools *typ.MCPSourceConfig
	var existingIndex = -1
	for i, source := range cfg.Sources {
		if source.ID == mcptools.BuiltinWebtoolsSourceID {
			existingWebtools = &cfg.Sources[i]
			existingIndex = i
			break
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
	isClientTool := true
	enabled := typ.BoolPtr(true)
	tools := mcptools.DefaultBuiltinWebtoolNames()
	if existingWebtools != nil {
		if existingWebtools.Enabled != nil {
			enabled = existingWebtools.Enabled
		}
		if existingWebtools.IsClientTool != nil {
			isClientTool = *existingWebtools.IsClientTool
		}
		if len(existingWebtools.Tools) > 0 {
			tools = existingWebtools.Tools
		}
	}
	builtinWebtools := typ.MCPSourceConfig{
		ID:           mcptools.BuiltinWebtoolsSourceID,
		Name:         mcptools.BuiltinWebtoolsSourceName,
		Transport:    "stdio",
		Command:      currentExecPath,         // Use absolute path of current binary
		Args:         []string{"mcp-builtin"}, // Subcommand to start builtin server
		Enabled:      enabled,
		IsClientTool: &isClientTool,
		Tools:        tools,
		Env:          preservedEnv, // Preserve user's environment variables
	}

	// Update or append the configuration
	if existingIndex >= 0 {
		cfg.Sources[existingIndex] = builtinWebtools
		logrus.Info("mcp: updated builtin webtools configuration")
	} else {
		cfg.Sources = append(cfg.Sources, builtinWebtools)
		logrus.Info("mcp: registered builtin webtools configuration")
	}

	if err := setConfig("mcp_runtime", cfg); err != nil {
		logrus.WithError(err).Error("mcp: failed to save builtin webtools configuration")
		return err
	}

	return nil
}
