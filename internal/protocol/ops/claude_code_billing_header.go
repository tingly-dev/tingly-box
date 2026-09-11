package ops

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/google/uuid"
)

// Native Claude Code identity for the claude_code_version rule flag.
//
// ApplyAnthropic{V1,Beta}MetadataTransform keep their historical (2.1.86)
// behavior when the flag is unset; when a supported version is selected they
// dispatch here instead. Everything below is reverse-engineered from the
// official 2.1.258 bundle and verified with live captures — see
// .design/claude-code-client-compat.md §3.3–§3.4.
//
// The x-anthropic-billing-header system block is rendered by the CLI as
//
//	x-anthropic-billing-header: cc_version=<ver>.<fp>; cc_entrypoint=<ep>;[ cch=00000;][ cc_workload=<w>;][ cc_is_subagent=true;][ cc_prev_req=<req_id>;][ cc_prompt_id=<uuid>;]
//
// where
//   - cc_version is the package version plus a 3-hex fingerprint of the
//     first user prompt (computeFingerprint);
//   - cc_entrypoint is CLAUDE_CODE_ENTRYPOINT ("cli" for the interactive
//     terminal, "sdk-cli" for -p / the Agent SDK, "remote" for CCR ...);
//   - cch=00000 is a placeholder the JS layer writes; the native (Bun/Zig)
//     layer of the official binary replaces it on the wire with a hash of
//     the outgoing body. tingly-box does the same in the Claude OAuth client
//     (internal/client/claude_cch.go), so this block must carry exactly the
//     placeholder;
//   - cc_workload, cc_is_subagent are emitted regardless of the base URL;
//   - cc_prev_req and cc_prompt_id are direct-only extras added after 2.1.86.
//
// tingly-box rebuilds the block for its pinned version (the inbound client may
// be an older CLI, an SDK entrypoint, or not Claude Code at all) but keeps the
// per-session fields a real client already attached.

// ClaudeCodeVersionExtraKey is the TransformContext.Extra key carrying the
// resolved claude_code_version flag (set by transform.ClaudeCodeVersionTransform).
const ClaudeCodeVersionExtraKey = "claude_code_version"

// ClaudeCodeVersionFromExtra returns the selected Claude Code version, or ""
// for the legacy path.
func ClaudeCodeVersionFromExtra(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	v, _ := extra[ClaudeCodeVersionExtraKey].(string)
	return v
}

const (
	// billingHeaderPrefix is the literal the CLI (and CleanHeaderTransform)
	// key on. The trailing space is part of the rendered format.
	billingHeaderPrefix = "x-anthropic-billing-header: "

	// claudeCodeEntrypoint is the persona tingly-box presents: the interactive
	// terminal. Must agree with the client's "(external, cli)" User-Agent.
	claudeCodeEntrypoint = "cli"

	// claudeCodeCCHPlaceholder is the JS-layer placeholder the client
	// middleware finds and patches with the body hash. It must not be
	// randomized here: the hash has to be computed over the final bytes.
	claudeCodeCCHPlaceholder = "00000"
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
// billing header.
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

// BuildClaudeCodeBillingHeader renders the billing header block. ccVersion is
// the full "<ver>.<fp>" value; existing is the inbound block (or "") whose
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
	b.WriteString(claudeCodeCCHPlaceholder)
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

// computeCCVersionFor is computeCCVersion for an explicit version.
func computeCCVersionFor(messageText, version string) string {
	return fmt.Sprintf("%s.%s", version, computeFingerprint(messageText, version))
}

// systemReminderPrefix opens the <system-reminder> blocks Claude Code attaches
// to a user turn (skills list, agent types, current date, ...). Inside the CLI
// those are separate "meta" messages; on the wire they are folded into the
// same user message ahead of the prompt the person typed.
const systemReminderPrefix = "<system-reminder>"

func isSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), systemReminderPrefix)
}

// extractFirstUserPromptText returns the text 2.1.258 fingerprints for
// cc_version: the first *non-meta* user message. On the wire that is the
// first text block of the first user message that is not a system reminder
// (2.1.86 still hashed the reminder itself — extractFirstUserMessageText; a
// 2.1.258 capture of "say hi" fingerprints to 8ee only with the prompt text).
// A user message made only of reminders is skipped; one with no text yields "".
func extractFirstUserPromptText(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		reminderOnly := false
		for _, block := range msg.Content {
			if block.OfText == nil {
				continue
			}
			if isSystemReminderText(block.OfText.Text) {
				reminderOnly = true
				continue
			}
			return block.OfText.Text
		}
		if !reminderOnly {
			return ""
		}
	}
	return ""
}

// extractFirstBetaUserPromptText is the beta-API twin of extractFirstUserPromptText.
func extractFirstBetaUserPromptText(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		reminderOnly := false
		for _, block := range msg.Content {
			if block.OfText == nil {
				continue
			}
			if isSystemReminderText(block.OfText.Text) {
				reminderOnly = true
				continue
			}
			return block.OfText.Text
		}
		if !reminderOnly {
			return ""
		}
	}
	return ""
}

// nativeMetadataUserID is metadata.user_id as 2.1.258 renders it:
//
//	{"device_id":"<64 hex>","account_uuid":"<uuid or empty>","session_id":"<uuid>"[,"parent_session_id":"<uuid>"]}
//
// Field order is what the CLI's JSON.stringify produces. parent_session_id is
// attached by subagent (Agent tool) sessions only and is passed through so a
// subagent request keeps its lineage after device/account are rewritten.
// (2.1.258 also has a remote-only "tk" key that never appears on a local CLI
// and is intentionally not modelled.)
type nativeMetadataUserID struct {
	DeviceID        string `json:"device_id"`
	AccountUUID     string `json:"account_uuid"`
	SessionID       string `json:"session_id"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
}

// buildNativeMetadataUserID rewrites the inbound user_id (JSON or legacy
// underscore form) with the gateway's device/account, keeping the client's
// session (and parent session). Returns "" when the gateway has no identity to
// stamp, in which case the field is left as the client sent it.
func buildNativeMetadataUserID(raw string, extra map[string]any) string {
	m := nativeMetadataUserID{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			if legacy := ParseMetadataUserID(raw); legacy != nil {
				m.DeviceID, m.AccountUUID, m.SessionID = legacy.DeviceID, legacy.AccountUUID, legacy.SessionID
			}
		}
	}
	if v, ok := extra["device"].(string); ok && v != "" {
		m.DeviceID = v
	}
	if v, ok := extra["user_id"].(string); ok && v != "" {
		m.AccountUUID = v
	}
	if m.DeviceID == "" || m.AccountUUID == "" {
		return ""
	}
	if m.SessionID == "" {
		m.SessionID = uuid.New().String()
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// applyNativeClaudeCodeIdentityV1 is the claude_code_version path of
// ApplyAnthropicV1MetadataTransform: billing header rebuilt in place for
// version (or prepended), metadata.user_id rewritten.
func applyNativeClaudeCodeIdentityV1(req *anthropic.MessageNewParams, extra map[string]any, version string) *anthropic.MessageNewParams {
	ccVersion := computeCCVersionFor(extractFirstUserPromptText(req.Messages), version)
	if len(req.System) > 0 && IsBillingHeaderText(req.System[0].Text) {
		req.System[0].Text = BuildClaudeCodeBillingHeader(ccVersion, req.System[0].Text)
	} else {
		req.System = append([]anthropic.TextBlockParam{{Text: BuildClaudeCodeBillingHeader(ccVersion, "")}}, req.System...)
	}
	if s := buildNativeMetadataUserID(req.Metadata.UserID.String(), extra); s != "" {
		req.Metadata.UserID = param.NewOpt(s)
	}
	return req
}

// applyNativeClaudeCodeIdentityBeta is the beta-API twin of applyNativeClaudeCodeIdentityV1.
func applyNativeClaudeCodeIdentityBeta(req *anthropic.BetaMessageNewParams, extra map[string]any, version string) *anthropic.BetaMessageNewParams {
	ccVersion := computeCCVersionFor(extractFirstBetaUserPromptText(req.Messages), version)
	if len(req.System) > 0 && IsBillingHeaderText(req.System[0].Text) {
		req.System[0].Text = BuildClaudeCodeBillingHeader(ccVersion, req.System[0].Text)
	} else {
		req.System = append([]anthropic.BetaTextBlockParam{{Text: BuildClaudeCodeBillingHeader(ccVersion, "")}}, req.System...)
	}
	if s := buildNativeMetadataUserID(req.Metadata.UserID.String(), extra); s != "" {
		req.Metadata.UserID = param.NewOpt(s)
	}
	return req
}
