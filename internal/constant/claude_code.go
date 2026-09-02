package constant

// ClaudeCodeVersion is the Claude Code release tingly-box impersonates on the
// Claude OAuth chain. It is the single source for every wire artifact that
// carries the version: the claude-cli User-Agent, the cc_version field of the
// x-anthropic-billing-header system block (and the fingerprint salted with
// it), and the "Claude Code (CLI)" custom_user_agent preset.
//
// Anthropic gates OAuth traffic on this version ("Claude Code X does not
// support this model; version Y or newer is required", error_code
// claude_code_version_too_old), so bumping it is a routine maintenance step.
// Bumping is not a one-line change though: the beta flags, stainless SDK
// headers, billing-header fields and metadata shape all drift between
// releases — re-derive them from the official package before moving this
// constant. The procedure and the current findings live in
// .design/claude-code-client-compat.md.
const ClaudeCodeVersion = "2.1.258"

// ClaudeCodeUserAgent returns the User-Agent a real interactive Claude Code
// CLI sends: "claude-cli/<version> (external, cli)". The second token is the
// entrypoint (CLAUDE_CODE_ENTRYPOINT); tingly-box always presents the
// interactive "cli" persona, matching cc_entrypoint=cli in the billing header.
func ClaudeCodeUserAgent() string {
	return "claude-cli/" + ClaudeCodeVersion + " (external, cli)"
}
