package client

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Native Claude Code profile (claude_code_version flag): overlays on the
// legacy client headers with what the official binary sends. See
// .design/claude-code.md Part B.

const (
	nativeStainlessRuntimeVersion = "v26.3.0" // Bun's Node-compat version
	nativeStainlessPackageVersion = "0.112.1"

	claudeRequestClassHeader = "x-claude-code-request-class"
	claudeAgentTypeHeader    = "x-claude-code-agent-type"
	claudeRequestClassMain   = "main"
	claudeXAppBackground     = "cli-bg" // x-app of a background session
)

// nativeClaudeCLIUserAgent matches cc_entrypoint=cli.
func nativeClaudeCLIUserAgent(version string) string {
	return "claude-cli/" + version + " (external, cli)"
}

// claudeCodeNativeVersion returns the selected native version, or "" for legacy.
func claudeCodeNativeVersion(ctx context.Context) string {
	v := typ.GetRuleFlags(ctx).ClaudeCodeVersion
	if typ.ClaudeCodeVersionEnabled(v) {
		return v
	}
	return ""
}

// claudeHintHeaderValueRe bounds replayed hint header values.
var claudeHintHeaderValueRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// stainlessOSName maps GOOS to the JS SDK's X-Stainless-OS value.
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

// stainlessArchName maps GOARCH to the JS SDK's X-Stainless-Arch value.
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

// applyNativeClaudeCodeHeaders overlays the client-level native headers. The
// CLI never uses the SDK's .stream() helper, so x-stainless-helper-method goes.
func applyNativeClaudeCodeHeaders(options []anthropicOption.RequestOption, version, model string, isOAuthToken bool) []anthropicOption.RequestOption {
	baseBetas := joinBetas(composeClaudeCodeBetas(claudeBetaSignals{Model: model, OAuth: isOAuthToken}))
	return append(options,
		anthropicOption.WithHeader("anthropic-beta", baseBetas),
		anthropicOption.WithHeader("user-agent", nativeClaudeCLIUserAgent(version)),
		anthropicOption.WithHeaderDel("x-stainless-helper-method"),
		anthropicOption.WithHeader("x-stainless-runtime-version", nativeStainlessRuntimeVersion),
		anthropicOption.WithHeader("x-stainless-package-version", nativeStainlessPackageVersion),
		anthropicOption.WithHeader("x-stainless-arch", stainlessArchName(runtime.GOARCH)),
		anthropicOption.WithHeader("x-stainless-os", stainlessOSName(runtime.GOOS)),
	)
}

// nativeRequestOptions returns the per-request overlays: composed betas,
// replayed client hints, and the cch middleware (claude_cch.go).
func (c *ClaudeClient) nativeRequestOptions(ctx context.Context, sig claudeBetaSignals) []anthropicOption.RequestOption {
	options := []anthropicOption.RequestOption{
		anthropicOption.WithHeader("anthropic-beta", joinBetas(composeClaudeCodeBetas(sig))),
		anthropicOption.WithMiddleware(claudeCodeCCHMiddleware),
	}
	hints := typ.GetClaudeCodeClientHints(ctx)
	if hints.BackgroundSession {
		options = append(options, anthropicOption.WithHeader("x-app", claudeXAppBackground))
	}
	if hints.AgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-agent-id", sanitizeClaudeHeaderValue(hints.AgentID)))
	}
	if hints.ParentAgentID != "" {
		options = append(options, anthropicOption.WithHeader("x-claude-code-parent-agent-id", sanitizeClaudeHeaderValue(hints.ParentAgentID)))
	}
	// Request class defaults to the interactive main thread.
	class := claudeRequestClassMain
	if claudeHintHeaderValueRe.MatchString(hints.RequestClass) {
		class = hints.RequestClass
	}
	options = append(options, anthropicOption.WithHeader(claudeRequestClassHeader, class))
	if claudeHintHeaderValueRe.MatchString(hints.AgentType) {
		options = append(options, anthropicOption.WithHeader(claudeAgentTypeHeader, hints.AgentType))
	}
	return options
}

// nativeCountTokensClient builds a client carrying the count_tokens beta subset.
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

// isOAuth reports whether the credential is a Claude OAuth token.
func (c *ClaudeClient) isOAuth() bool {
	return IsClaudeOAuthToken(c.AnthropicClient.provider.GetAccessToken())
}

// sanitizeClaudeHeaderValue percent-encodes '%' and non-printable bytes, like the CLI.
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
