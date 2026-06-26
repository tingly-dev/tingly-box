package client

import (
	"runtime"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// knownAnthropicBetas is the universe of valid anthropic-beta flag values
// the upstream API recognizes — sourced from the SDK's AnthropicBeta*
// constants plus Claude Code specific flags Anthropic ships ahead of the
// public SDK. Anything outside this set is treated as garbage at the
// outermost layer (likely a buggy or hostile caller).
var knownAnthropicBetas = func() map[string]struct{} {
	flags := []string{
		// SDK-defined (libs/anthropic-sdk-go/beta.go)
		anthropic.AnthropicBetaMessageBatches2024_09_24,
		anthropic.AnthropicBetaPromptCaching2024_07_31,
		anthropic.AnthropicBetaComputerUse2024_10_22,
		anthropic.AnthropicBetaComputerUse2025_01_24,
		anthropic.AnthropicBetaPDFs2024_09_25,
		anthropic.AnthropicBetaTokenCounting2024_11_01,
		anthropic.AnthropicBetaTokenEfficientTools2025_02_19,
		anthropic.AnthropicBetaOutput128k2025_02_19,
		anthropic.AnthropicBetaFilesAPI2025_04_14,
		anthropic.AnthropicBetaMCPClient2025_04_04,
		anthropic.AnthropicBetaMCPClient2025_11_20,
		anthropic.AnthropicBetaDevFullThinking2025_05_14,
		anthropic.AnthropicBetaInterleavedThinking2025_05_14,
		anthropic.AnthropicBetaCodeExecution2025_05_22,
		anthropic.AnthropicBetaExtendedCacheTTL2025_04_11,
		anthropic.AnthropicBetaContext1m2025_08_07,
		anthropic.AnthropicBetaContextManagement2025_06_27,
		anthropic.AnthropicBetaModelContextWindowExceeded2025_08_26,
		anthropic.AnthropicBetaSkills2025_10_02,
		anthropic.AnthropicBetaFastMode2026_02_01,
		anthropic.AnthropicBetaOutput300k2026_03_24,
		anthropic.AnthropicBetaUserProfiles2026_03_24,
		anthropic.AnthropicBetaAdvisorTool2026_03_01,
		anthropic.AnthropicBetaManagedAgents2026_04_01,
		anthropic.AnthropicBetaCacheDiagnosis2026_04_07,
		anthropic.AnthropicBetaThinkingTokenCount2026_05_13,
		// Claude Code specific flags not (yet) exposed by the SDK
		"claude-code-20250219",
		"oauth-2025-04-20",
		"prompt-caching-scope-2026-01-05",
		"structured-outputs-2025-12-15",
		"redact-thinking-2026-02-12",
		"token-efficient-tools-2026-03-28",
		"oidc-federation-2026-04-01",
	}
	m := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		m[f] = struct{}{}
	}
	return m
}()

// claudeCodeRequiredBetasOrdered is the required baseline as an ordered
// slice, derived once from anthropicBeta. mergeBetaFlags iterates this on
// every request instead of re-splitting the constant per call.
var claudeCodeRequiredBetasOrdered = func() []string {
	parts := strings.Split(anthropicBeta, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}()

// claudeCodeRequiredBetas is the set form of the same baseline, for O(1)
// membership checks in classifyUpstreamBetaFlag.
var claudeCodeRequiredBetas = func() map[string]struct{} {
	m := make(map[string]struct{}, len(claudeCodeRequiredBetasOrdered))
	for _, p := range claudeCodeRequiredBetasOrdered {
		m[p] = struct{}{}
	}
	return m
}()

// claudeCodeAllowedUpstreamBetas is the very narrow set of anthropic-beta
// flags we accept FROM upstream callers on top of the required baseline.
//
// Why this is restrictive: Anthropic fingerprints Claude Code OAuth
// traffic, and `anthropic-beta` is one of the signals. Forwarding any
// SDK-known flag (e.g. message-batches, managed-agents, pdfs, mcp-client)
// would emit a header shape no real claude-cli ever sends, which both
// breaks fingerprinting and may trigger anti-abuse responses.
//
// Only flags that real Claude Code is known to add conditionally — or
// that have been explicitly cleared as fingerprint-safe — belong here.
// When in doubt, leave it out.
var claudeCodeAllowedUpstreamBetas = map[string]struct{}{
	// Model-conditional 1M context window; real Claude Code adds this
	// for sonnet/opus, so accepting it from upstream is safe.
	anthropic.AnthropicBetaContext1m2025_08_07: {},
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
)

// stainlessOS returns the OS name for the x-stainless-os header.
func stainlessOS() string {
	return runtime.GOOS // e.g., "darwin", "linux", "windows"
}

// stainlessArch returns the architecture for the x-stainless-arch header.
func stainlessArch() string {
	return runtime.GOARCH // e.g., "amd64", "arm64"
}

// claudeModelPrefixes that support context-1m beta flag.
var context1mModelPrefixes = []string{
	"claude-sonnet-4-6",
	"claude-opus-4-6",
}

// supportsContext1M checks if the model supports the context-1m-2025-08-07 beta flag.
func supportsContext1M(model string) bool {
	m := strings.ToLower(model)
	for _, prefix := range context1mModelPrefixes {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}
