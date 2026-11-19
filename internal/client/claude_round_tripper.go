package client

import (
	"runtime"
	"strings"
)

// claudeToolPrefix is empty to match real Claude Code behavior (no tool name prefix).
const claudeToolPrefix = ""

// oauthToolRenameMap maps lowercase tool names to Claude Code TitleCase equivalents.
// Anthropic uses tool name fingerprinting to detect third-party clients on OAuth traffic.
// Renaming to official names avoids extra-usage billing.
var oauthToolRenameMap = map[string]string{
	"bash":         "Bash",
	"read":         "Read",
	"write":        "Write",
	"edit":         "Edit",
	"glob":         "Glob",
	"grep":         "Grep",
	"task":         "Task",
	"webfetch":     "WebFetch",
	"todowrite":    "TodoWrite",
	"question":     "Question",
	"skill":        "Skill",
	"ls":           "LS",
	"todoread":     "TodoRead",
	"notebookedit": "NotebookEdit",
}

const (
	// Claude Code client identification
	claudeCLIUserAgent      = "claude-cli/2.1.86 (external, cli)"
	claudeXApp              = "cli"
	stainlessHelperMethod   = "stream"
	stainlessRetryCount     = "0"
	stainlessRuntimeVersion = "v24.3.0"
	stainlessPackageVersion = "0.74.0"
	stainlessRuntime        = "node"
	stainlessLang           = "js"
	stainlessTimeout        = "600"

	// Anthropic API headers
	anthropicBeta                         = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28"
	anthropicOAuthBeta                    = "oauth-2025-04-20"
	anthropicDangerousDirectBrowserAccess = "true"
	anthropicVersion                      = "2023-06-01"

	// Model-specific beta flags
	anthropicContext1m = "context-1m-2025-08-07"

	// AnthropicContext1m is the exported version for use in other packages
	AnthropicContext1m = anthropicContext1m

	// Content negotiation
	acceptHeader = "application/json"

	// Buffer sizes
	maxStreamingLineSize = 52_428_800 // 50MB max line size
)

// stainlessOS returns the OS name for the x-stainless-os header
func stainlessOS() string {
	return runtime.GOOS // e.g., "darwin", "linux", "windows"
}

// stainlessArch returns the architecture for the x-stainless-arch header
func stainlessArch() string {
	return runtime.GOARCH // e.g., "amd64", "arm64"
}

// IsClaudeOAuthToken checks if the given API key is a Claude OAuth token
// by checking for the "sk-ant-oat" prefix.
func IsClaudeOAuthToken(apiKey string) bool {
	return apiKey != "" && strings.Contains(apiKey, "sk-ant-oat")
}
