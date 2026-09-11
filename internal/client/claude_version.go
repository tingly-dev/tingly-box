package client

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Native Claude Code profile for the claude_code_version rule flag.
//
// With the flag unset, ClaudeClient keeps the historical 2.1.86 emulation
// (claude_round_tripper.go constants, static anthropic-beta list). With
// typ.ClaudeCodeVersion2_1_258 selected, the overlays below replace the
// version-bound pieces with what the official 2.1.258 native binary sends —
// values taken from its bundle and confirmed by live captures against a fake
// API (.design/claude-code-client-compat.md §2, §3.1).

const (
	// nativeClaudeCLIUserAgent is "claude-cli/<version> (external, cli)": the
	// interactive terminal entrypoint, matching cc_entrypoint=cli.
	nativeClaudeCLIUserAgent = "claude-cli/" + typ.ClaudeCodeVersion2_1_258 + " (external, cli)"

	// Since 2.1.251 the CLI ships as a native Bun binary, so the "node"
	// runtime reports Bun's Node-compat version; the bundled @anthropic-ai/sdk
	// moved to 0.112.1.
	nativeStainlessRuntimeVersion = "v26.3.0" // Bun 1.4.1 (2.1.258 binary)
	nativeStainlessPackageVersion = "0.112.1"
)

// claudeCodeNative reports whether the request's resolved rule flags select
// the native profile.
func claudeCodeNative(ctx context.Context) bool {
	return typ.ClaudeCodeVersionEnabled(typ.GetRuleFlags(ctx).ClaudeCodeVersion)
}

// stainlessOSName maps a GOOS to the X-Stainless-OS value the JS SDK derives
// from process.platform ("MacOS", "Linux", "Windows", ...).
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

// stainlessArchName maps a GOARCH to the X-Stainless-Arch value the JS SDK
// derives from process.arch ("x64", "arm64", ...).
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

// applyNativeClaudeCodeHeaders overlays the native profile on the legacy
// client-level headers (WithHeader replaces, WithHeaderDel removes): the
// version-bound UA and SDK triple, SDK display names for OS/arch, no
// x-stainless-helper-method (the CLI calls messages.create({stream:true})
// directly, never the .stream() helper), and the model-dependent
// anthropic-beta baseline. Request-scoped flags are added per call in
// nativeRequestOptions.
func applyNativeClaudeCodeHeaders(options []anthropicOption.RequestOption, model string, isOAuthToken bool) []anthropicOption.RequestOption {
	baseBetas := joinBetas(composeClaudeCodeBetas(claudeBetaSignals{Model: model, OAuth: isOAuthToken}))
	return append(options,
		anthropicOption.WithHeader("anthropic-beta", baseBetas),
		anthropicOption.WithHeader("user-agent", nativeClaudeCLIUserAgent),
		anthropicOption.WithHeaderDel("x-stainless-helper-method"),
		anthropicOption.WithHeader("x-stainless-runtime-version", nativeStainlessRuntimeVersion),
		anthropicOption.WithHeader("x-stainless-package-version", nativeStainlessPackageVersion),
		anthropicOption.WithHeader("x-stainless-arch", stainlessArchName(runtime.GOARCH)),
		anthropicOption.WithHeader("x-stainless-os", stainlessOSName(runtime.GOOS)),
	)
}

// nativeRequestOptions returns the per-request overlays of the native
// profile: the composed anthropic-beta list (overriding the client-level
// baseline), the subagent lineage headers replayed from the inbound client,
// and — last stop before the wire — the JS-canonical JSON + cch body hash
// middleware (claude_cch.go), mirroring the official binary's native layer.
func (c *ClaudeClient) nativeRequestOptions(ctx context.Context, sig claudeBetaSignals) []anthropicOption.RequestOption {
	options := []anthropicOption.RequestOption{
		anthropicOption.WithHeader("anthropic-beta", joinBetas(composeClaudeCodeBetas(sig))),
		anthropicOption.WithMiddleware(claudeCodeCCHMiddleware),
	}
	hints := typ.GetClaudeCodeClientHints(ctx)
	if hints.AgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-agent-id", sanitizeClaudeHeaderValue(hints.AgentID)))
	}
	if hints.ParentAgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-parent-agent-id", sanitizeClaudeHeaderValue(hints.ParentAgentID)))
	}
	return options
}

// nativeCountTokensClient builds a client whose anthropic-beta header is the
// count_tokens subset the CLI sends (claude-code, interleaved-thinking,
// context-management, oauth) for model.
func (c *ClaudeClient) nativeCountTokensClient(ctx context.Context, model string) anthropic.Client {
	sig := baseClaudeBetaSignals(ctx, model, c.isOAuth())
	betas := filterClaudeCodeCountTokensBetas(composeClaudeCodeBetas(sig))
	base := c.AnthropicClient.Client().Options
	options := make([]anthropicOption.RequestOption, 0, len(base)+2)
	options = append(options, base...)
	options = append(options,
		anthropicOption.WithHeader("anthropic-beta", joinBetas(betas)),
		anthropicOption.WithMiddleware(claudeCodeCCHMiddleware),
	)
	return anthropic.NewClient(options...)
}

// isOAuth reports whether the provider credential is a Claude OAuth token
// (the oauth-2025-04-20 beta rides only on those).
func (c *ClaudeClient) isOAuth() bool {
	return IsClaudeOAuthToken(c.AnthropicClient.provider.GetAccessToken())
}

// sanitizeClaudeHeaderValue reproduces the CLI's agent-id header encoder:
// '%' and every byte outside printable ASCII are percent-encoded so the value
// is always a valid HTTP header value.
func sanitizeClaudeHeaderValue(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch == '%' || ch < 0x20 || ch > 0x7e {
			fmt.Fprintf(&b, "%%%02X", ch)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
