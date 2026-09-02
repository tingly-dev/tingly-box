package ops

import (
	"regexp"
	"strings"
)

// Claude Code's x-anthropic-billing-header system block.
//
// Reverse-engineered from the official @anthropic-ai/claude-code 2.1.258
// bundle (see .design/claude-code-client-compat.md §4). The CLI renders the
// block as
//
//	x-anthropic-billing-header: cc_version=<ver>.<fp>; cc_entrypoint=<ep>;[ cch=00000;][ cc_workload=<w>;][ cc_is_subagent=true;][ cc_prev_req=<req_id>;][ cc_prompt_id=<uuid>;]
//
// where
//   - cc_version is the package version plus a 3-hex fingerprint of the
//     first user prompt (computeCCVersion);
//   - cc_entrypoint is CLAUDE_CODE_ENTRYPOINT ("cli" for the interactive
//     terminal, "sdk-cli" for -p / the Agent SDK, "remote" for CCR ...);
//   - cch=00000 is a constant placeholder, emitted only when the CLI believes
//     it talks to api.anthropic.com directly (or to Vertex);
//   - cc_workload, cc_is_subagent are emitted regardless of the base URL:
//     the workload is an AsyncLocalStorage tag (e.g. "cron"), the subagent
//     flag marks Agent-tool (subagent) sessions;
//   - cc_prev_req (the previous response's request-id) and cc_prompt_id (a
//     per-user-turn UUID) are direct-only extras added after 2.1.86.
//
// tingly-box rebuilds the block for its pinned version (the inbound client may
// be an older CLI, an SDK entrypoint, or not Claude Code at all) but keeps the
// per-session fields a real client already attached, so a subagent request
// stays a subagent request and a client that was told to assume a first-party
// base URL keeps its prompt/request correlation.

const (
	// billingHeaderPrefix is the literal the CLI (and CleanHeaderTransform)
	// key on. The trailing space is part of the rendered format.
	billingHeaderPrefix = "x-anthropic-billing-header: "

	// claudeCodeEntrypoint is the persona tingly-box presents: the interactive
	// terminal. Must agree with constant.ClaudeCodeUserAgent ("(external, cli)").
	claudeCodeEntrypoint = "cli"

	// claudeCodeCCH is the constant cache-hash placeholder the CLI sends on
	// direct first-party traffic. It has been "00000" in every release
	// inspected (2.1.86 sent it unconditionally, 2.1.258 gates it on the base
	// URL); it is not random and must not be randomized.
	claudeCodeCCH = "00000"
)

// billingHeaderPreservedField describes a field a real client attaches that
// tingly-box passes through unchanged, in the order the CLI renders them.
type billingHeaderPreservedField struct {
	key   string
	valid *regexp.Regexp
}

// billingHeaderPreservedFields is the ordered pass-through allowlist. The
// validators mirror the CLI's own guards (cc_prev_req / cc_prompt_id are
// regex-checked before emission) or the value space the CLI can produce, so a
// malformed inbound block can never smuggle arbitrary text into the header
// tingly-box vouches for.
var billingHeaderPreservedFields = []billingHeaderPreservedField{
	{key: "cc_workload", valid: regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)},
	{key: "cc_is_subagent", valid: regexp.MustCompile(`^true$`)},
	{key: "cc_prev_req", valid: regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,36}$`)},
	{key: "cc_prompt_id", valid: regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)},
}

// IsBillingHeaderText reports whether a system block carries Claude Code's
// billing header. Shared by the injector (which replaces the block) and the
// clean_header transform (which strips it).
func IsBillingHeaderText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "x-anthropic-billing-header:")
}

// parseBillingHeaderFields splits a billing header block into its key/value
// pairs, in order. Tolerates a missing prefix and sloppy whitespace; a
// segment without "=" is dropped.
func parseBillingHeaderFields(text string) [][2]string {
	body := strings.TrimSpace(text)
	if i := strings.Index(body, ":"); i >= 0 && strings.HasPrefix(body, "x-anthropic-billing-header") {
		body = body[i+1:]
	}
	var fields [][2]string
	for _, seg := range strings.Split(body, ";") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		k, v, ok := strings.Cut(seg, "=")
		if !ok {
			continue
		}
		fields = append(fields, [2]string{strings.TrimSpace(k), strings.TrimSpace(v)})
	}
	return fields
}

// BuildClaudeCodeBillingHeader renders the billing header block for the
// pinned Claude Code version. ccVersion is the full "<ver>.<fp>" value
// (computeCCVersion); existing is the inbound block (or "") whose
// pass-through fields are preserved.
//
// Layout follows the 2.1.258 renderer field-for-field:
//
//	cc_version; cc_entrypoint; cch; cc_workload; cc_is_subagent; cc_prev_req; cc_prompt_id
func BuildClaudeCodeBillingHeader(ccVersion, existing string) string {
	var b strings.Builder
	b.WriteString(billingHeaderPrefix)
	b.WriteString("cc_version=")
	b.WriteString(ccVersion)
	b.WriteString("; cc_entrypoint=")
	b.WriteString(claudeCodeEntrypoint)
	b.WriteString("; cch=")
	b.WriteString(claudeCodeCCH)
	b.WriteString(";")

	if existing == "" {
		return b.String()
	}
	inbound := map[string]string{}
	for _, kv := range parseBillingHeaderFields(existing) {
		if _, dup := inbound[kv[0]]; !dup {
			inbound[kv[0]] = kv[1]
		}
	}
	for _, f := range billingHeaderPreservedFields {
		v, ok := inbound[f.key]
		if !ok || !f.valid.MatchString(v) {
			continue
		}
		b.WriteString(" ")
		b.WriteString(f.key)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString(";")
	}
	return b.String()
}
