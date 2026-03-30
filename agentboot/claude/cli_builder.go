package claude

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// CommonOptions represents per-execution options that can override config
type CommonOptions struct {
	Model                string
	FallbackModel        string
	MaxTurns             int
	CustomSystemPrompt   string
	AppendSystemPrompt   string
	ContinueConversation bool
	Resume               string
	AllowedTools         []string
	DisallowedTools      []string
	MCPServers           map[string]interface{}
	StrictMcpConfig      bool
	PermissionMode       string
	SettingsPath         string

	// PermissionPromptTool specifies the tool for permission prompts (e.g., "stdio")
	PermissionPromptTool string
}

// BuildCommonArgs builds CLI arguments shared between Launcher and QueryLauncher
// This follows the original query_launcher.go behavior where config and opts
// arguments are added independently (both can be present).
func BuildCommonArgs(config Config, opts CommonOptions) []string {
	var args []string

	// Model selection - both config and opts can contribute (opts takes precedence in practice)
	if config.Model != "" {
		args = append(args, "--model", config.Model)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// Fallback model - only from opts (config doesn't have this in original)
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	// Max turns
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
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

	// Permission mode - opts overrides config, emit only once
	mode := string(config.PermissionMode)
	if opts.PermissionMode != "" {
		mode = opts.PermissionMode
	}
	if mode != "" {
		if !IsValidPermissionMode(mode) {
			log.Printf("[WARN] invalid permission mode %q, skipping --permission-mode flag", mode)
		} else {
			args = append(args, "--permission-mode", mode)
		}
	}

	// Conversation control - both config and opts can set --continue
	if config.ContinueConversation {
		args = append(args, "--continue")
	}
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}

	// Resume session - config first, then opts (opts may override)
	if config.ResumeSessionID != "" {
		args = append(args, "--resume", config.ResumeSessionID)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	// Tool filtering - both config and opts can contribute
	if len(config.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(config.AllowedTools, ","))
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	if len(config.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(config.DisallowedTools, ","))
	}
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}

	// MCP servers - merge from config and opts
	args = append(args, buildMCPArgs(config, opts)...)

	// Settings path - both config and opts can contribute
	if config.SettingsPath != "" {
		args = append(args, "--settings", config.SettingsPath)
	}
	if opts.SettingsPath != "" {
		args = append(args, "--settings", opts.SettingsPath)
	}

	// Permission prompt tool (e.g., "stdio" for callback-based permission handling)
	if opts.PermissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptTool)
	}

	return args
}

// buildMCPArgs builds MCP server arguments from config and opts
func buildMCPArgs(config Config, opts CommonOptions) []string {
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
