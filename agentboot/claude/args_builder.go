package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CommonOptions represents per-execution options that can override config
type CommonOptions struct {
	Model               string
	FallbackModel       string
	MaxTurns            int
	CustomSystemPrompt  string
	AppendSystemPrompt  string
	ContinueConversation bool
	Resume              string
	AllowedTools        []string
	DisallowedTools     []string
	MCPServers          map[string]interface{}
	StrictMcpConfig     bool
	PermissionMode      string
	SettingsPath        string
}

// BuildCommonArgs builds CLI arguments shared between Launcher and QueryLauncher
// Opts fields take precedence over config fields when both are set
func BuildCommonArgs(config Config, opts CommonOptions) []string {
	var args []string

	// Model selection - opts override config
	model := opts.Model
	if model == "" {
		model = config.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	fallbackModel := opts.FallbackModel
	if fallbackModel == "" {
		fallbackModel = config.FallbackModel
	}
	if fallbackModel != "" {
		args = append(args, "--fallback-model", fallbackModel)
	}

	// Max turns - opts override config
	maxTurns := opts.MaxTurns
	if maxTurns == 0 {
		maxTurns = config.MaxTurns
	}
	if maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", maxTurns))
	}

	// System prompts - both config and opts can contribute
	if config.CustomSystemPrompt != "" {
		args = append(args, "--system-prompt", config.CustomSystemPrompt)
	}
	if opts.CustomSystemPrompt != "" {
		args = append(args, "--system-prompt", opts.CustomSystemPrompt)
	}
	if config.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", config.AppendSystemPrompt)
	}
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}

	// Conversation control
	if config.ContinueConversation || opts.ContinueConversation {
		args = append(args, "--continue")
	}

	resume := opts.Resume
	if resume == "" {
		resume = config.ResumeSessionID
	}
	if resume != "" {
		args = append(args, "--resume", resume)
	}

	// Tool filtering
	args = append(args, buildToolArgs(config, opts)...)

	// MCP servers
	args = append(args, buildMCPArgsFromCommon(config, opts)...)

	// Permission mode
	if config.PermissionMode != "" {
		args = append(args, "--permission-mode", string(config.PermissionMode))
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}

	// Settings path - opts override config
	settingsPath := opts.SettingsPath
	if settingsPath == "" {
		settingsPath = config.SettingsPath
	}
	if settingsPath != "" {
		args = append(args, "--settings", settingsPath)
	}

	return args
}

// buildToolArgs builds tool filtering arguments
func buildToolArgs(config Config, opts CommonOptions) []string {
	var args []string

	// Merge allowed tools from config and opts
	allowedTools := config.AllowedTools
	if len(opts.AllowedTools) > 0 {
		// Opts override config if specified
		allowedTools = opts.AllowedTools
	}
	if len(allowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowedTools, ","))
	}

	// Merge disallowed tools from config and opts
	disallowedTools := config.DisallowedTools
	if len(opts.DisallowedTools) > 0 {
		// Opts override config if specified
		disallowedTools = opts.DisallowedTools
	}
	if len(disallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(disallowedTools, ","))
	}

	return args
}

// buildMCPArgsFromCommon builds MCP server arguments from config and opts
func buildMCPArgsFromCommon(config Config, opts CommonOptions) []string {
	var args []string

	// Merge MCP servers from config and opts
	mcpServers := make(map[string]interface{})
	if config.MCPServers != nil {
		for k, v := range config.MCPServers {
			mcpServers[k] = v
		}
	}
	if opts.MCPServers != nil {
		for k, v := range opts.MCPServers {
			mcpServers[k] = v
		}
	}

	if len(mcpServers) > 0 {
		mcpConfig := map[string]interface{}{"mcpServers": mcpServers}
		mcpJSON, _ := json.Marshal(mcpConfig)
		args = append(args, "--mcp-config", string(mcpJSON))
	}

	// Strict MCP config
	if config.StrictMcpConfig || opts.StrictMcpConfig {
		args = append(args, "--strict-mcp-config")
	}

	return args
}
