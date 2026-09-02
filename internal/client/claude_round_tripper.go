package client

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// Claude Code OAuth chain: pinned client identity.
//
// Every value below is what the official Claude Code release named by
// constant.ClaudeCodeVersion puts on the wire, taken from the native binary's
// bundle and confirmed by live captures against a fake API server
// (.design/claude-code-client-compat.md §2). Keep them in lock-step with the
// version: the server rejects a stale version outright
// (claude_code_version_too_old), and a mismatched SDK/runtime triple is a
// fingerprint tell.

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
	// Claude Code client identification. The CLI sets x-app ("cli", or
	// "cli-bg" for background sessions), User-Agent and
	// X-Claude-Code-Session-Id itself; the X-Stainless-* set comes from the
	// bundled @anthropic-ai/sdk. Since 2.1.251 the CLI ships as a native Bun
	// binary, so the "node" runtime reports Bun's Node-compat version.
	claudeXApp              = "cli"
	stainlessRetryCount     = "0"
	stainlessRuntimeVersion = "v26.3.0" // Bun 1.4.1 (2.1.258 binary)
	stainlessPackageVersion = "0.112.1" // @anthropic-ai/sdk bundled in 2.1.258
	stainlessRuntime        = "node"
	stainlessLang           = "js"
	stainlessTimeout        = "600" // API_TIMEOUT_MS default 600000 / 1000

	// Anthropic API headers
	anthropicOAuthBeta                    = betaOAuth
	anthropicDangerousDirectBrowserAccess = "true"
	anthropicVersion                      = "2023-06-01"

	// Model-specific beta flags
	anthropicContext1m = betaContext1M

	// AnthropicContext1m is the exported version for use in other packages
	AnthropicContext1m = anthropicContext1m

	// Content negotiation
	acceptHeader = "application/json"

	// Buffer sizes
	maxStreamingLineSize = 52_428_800 // 50MB max line size
)

// claudeCLIUserAgent is the pinned User-Agent ("claude-cli/<ver> (external, cli)").
var claudeCLIUserAgent = constant.ClaudeCodeUserAgent()

// stainlessOS returns the X-Stainless-OS value the JS SDK derives from
// process.platform ("MacOS", "Linux", "Windows", ...).
func stainlessOS() string {
	return stainlessOSName(runtime.GOOS)
}

func stainlessOSName(goos string) string {
	switch goos {
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "android":
		return "Android"
	case "ios":
		return "iOS"
	case "":
		return "Unknown"
	default:
		return fmt.Sprintf("Other:%s", goos)
	}
}

// stainlessArch returns the X-Stainless-Arch value the JS SDK derives from
// process.arch ("x64", "arm64", ...).
func stainlessArch() string {
	return stainlessArchName(runtime.GOARCH)
}

func stainlessArchName(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x32"
	case "arm":
		return "arm"
	case "":
		return "unknown"
	default:
		return fmt.Sprintf("other:%s", goarch)
	}
}

// IsClaudeOAuthToken checks if the given API key is a Claude OAuth token
// by checking for the "sk-ant-oat" prefix.
func IsClaudeOAuthToken(apiKey string) bool {
	return apiKey != "" && strings.Contains(apiKey, "sk-ant-oat")
}
