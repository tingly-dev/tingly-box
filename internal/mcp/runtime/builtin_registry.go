package runtime

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RegisterBuiltinTools ensures built-in tools are registered in the MCP configuration.
// It checks if the webtools source already exists, and if not, adds it to the configuration.
func RegisterBuiltinTools(getConfig func() *typ.MCPRuntimeConfig, setConfig func(toolType string, config interface{}) error) error {
	cfg := getConfig()
	if cfg == nil {
		cfg = &typ.MCPRuntimeConfig{}
	}

	// Check if webtools already exists
	for _, source := range cfg.Sources {
		if source.ID == "webtools" {
			logrus.Info("mcp: builtin webtools already registered, skipping auto-registration")
			return nil
		}
	}

	// Auto-register built-in webtools as a stdio MCP server
	isClientTool := true
	builtinWebtools := typ.MCPSourceConfig{
		ID:           "webtools",
		Name:         "Built-in Web Tools",
		Transport:    "stdio",
		Command:      "tingly-box",           // Main binary
		Args:         []string{"mcp-builtin"}, // Subcommand to start builtin server
		Enabled:      typ.BoolPtr(true),
		IsClientTool: &isClientTool,
		Tools:        []string{"mcp_web_search", "mcp_web_fetch"},
		Env: map[string]string{
			"SERPER_API_KEY": "${SERPER_API_KEY}", // User provides via UI
		},
	}

	cfg.Sources = append(cfg.Sources, builtinWebtools)

	if err := setConfig("mcp_runtime", cfg); err != nil {
		logrus.WithError(err).Error("mcp: failed to save builtin webtools configuration")
		return err
	}

	logrus.Info("mcp: builtin webtools auto-registered successfully")
	return nil
}
