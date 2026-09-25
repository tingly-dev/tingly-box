package ops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/protocol/metaid"
)

// Native Claude Code identity (claude_code_version flag): rebuilds the
// x-anthropic-billing-header block and metadata.user_id the way the CLI
// renders them, keeping the per-session fields a real client attached. The
// legacy path is untouched. See .design/claude-code.md Part B.

// ClaudeCodeVersionExtraKey carries the flag in TransformContext.Extra.
const ClaudeCodeVersionExtraKey = "claude_code_version"

// ClaudeCodeVersionFromExtra returns the selected version, or "" for legacy.
func ClaudeCodeVersionFromExtra(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	v, _ := extra[ClaudeCodeVersionExtraKey].(string)
	return v
}

const (
	billingHeaderPrefix      = "x-anthropic-billing-header: "
	claudeCodeEntrypoint     = "cli"   // matches the "(external, cli)" User-Agent
	claudeCodeCCHPlaceholder = "00000" // patched with the body hash by the client middleware
)

// billingHeaderPreservedFields are the inbound fields passed through, in the
// CLI's render order. Validators mirror the CLI's own guards so a malformed
// inbound block cannot inject text into the header.
var billingHeaderPreservedFields = []struct {
	key   string
	valid *regexp.Regexp
}{
	{"cc_workload", regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)},
	{"cc_is_subagent", regexp.MustCompile(`^true$`)},
	{"cc_prev_req", regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,36}$`)},
	{"cc_prompt_id", regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)},
	{"cc_turn_origin", regexp.MustCompile(`^[a-z][a-z_]{0,31}$`)},
}

// IsBillingHeaderText reports whether a system block is the billing header.
func IsBillingHeaderText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "x-anthropic-billing-header:")
}

// parseBillingHeaderFields splits a billing header block into ordered
// key/value pairs; segments without "=" are dropped.
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
// "<ver>.<fp>"; existing is the inbound block (or "") whose preserved fields
// are kept.
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

// computeCCVersionFor renders "<version>.<fp>" for the native profile.
func computeCCVersionFor(messageText, version string) string {
	return fmt.Sprintf("%s.%s", version, computeFingerprintJS(messageText, version))
}

// computeFingerprintJS is computeFingerprint with the CLI's JavaScript string
// semantics: text[i] indexes UTF-16 code units, and the hash input is UTF-8
// with a lone surrogate encoded as U+FFFD (Node's behavior). Identical to
// computeFingerprint for ASCII; the legacy path keeps the byte version.
func computeFingerprintJS(messageText, version string) string {
	units := utf16.Encode([]rune(messageText))
	var chars strings.Builder
	for _, i := range []int{4, 7, 20} {
		if i >= len(units) {
			chars.WriteByte('0')
			continue
		}
		r := rune(units[i])
		if utf16.IsSurrogate(r) {
			r = utf8.RuneError
		}
		chars.WriteRune(r)
	}
	sum := sha256.Sum256([]byte(FingerprintSalt + chars.String() + version))
	return fmt.Sprintf("%x", sum[:2])[:3]
}

// systemReminderPrefix opens the meta blocks the CLI folds into a user turn.
const systemReminderPrefix = "<system-reminder>"

func isSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), systemReminderPrefix)
}

// extractFirstUserPromptText returns the text the fingerprint covers: the
// first user text block that is not a system reminder. (Legacy 2.1.86 hashed
// the reminder itself.)
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

// nativeMetadataUserID is metadata.user_id in the CLI's field order.
// parent_session_id comes from subagent sessions and is passed through.
type nativeMetadataUserID struct {
	DeviceID        string `json:"device_id"`
	AccountUUID     string `json:"account_uuid"`
	SessionID       string `json:"session_id"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
}

// buildNativeMetadataUserID stamps the gateway's device/account onto the
// inbound user_id, keeping the client's sessions. Returns "" when the gateway
// has no identity, leaving the client's value in place.
func buildNativeMetadataUserID(raw string, extra map[string]any) string {
	m := nativeMetadataUserID{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			if legacy := metaid.ParseMetadataUserID(raw); legacy != nil {
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

// applyNativeClaudeCodeIdentityV1 rebuilds (or prepends) the billing header
// and rewrites metadata.user_id.
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
