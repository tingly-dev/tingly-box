package protocoltest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestClaudeCodeIdentity_EndToEnd drives one Claude-Code-shaped request through
// the real gateway (claude_code scenario → Claude OAuth provider) and asserts
// the whole re-signed identity on the upstream wire, as captured from the
// official 2.1.258 client (.design/claude-code-client-compat.md):
//
//   - client headers: pinned UA / SDK triple, single composed anthropic-beta
//     value with the inbound per-turn flag replayed and the foreign flag
//     dropped, subagent id headers forwarded, no helper-method header;
//   - system header: billing header rebuilt in place for the pinned version
//     with the fingerprint of the user prompt (system reminders skipped) and
//     the client's cc_is_subagent preserved, cch constant;
//   - metadata: device/account rewritten, session and parent session kept.
func TestClaudeCodeIdentity_EndToEnd(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Close()

	s := flagScenario()
	env.virtual.RegisterScenario(s)
	const providerName = "identity-claude"
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID:     providerName,
		Name:     providerName,
		APIBase:  env.virtual.URL(),
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			Issuer:      ai.IssuerClaudeCode,
			AccessToken: "sk-ant-oat01-virtual",
		},
		Enabled: true,
		Timeout: int64(constant.DefaultRequestTimeout),
	})
	providerModel := "virtual-model-" + s.Name
	const reqModel = "pv-identity"
	rule := newHarnessRule(reqModel, typ.ScenarioClaudeCode, reqModel, providerModel,
		harnessService(providerName, providerModel))
	_ = env.appConfig.GetGlobalConfig().AddRequestConfig(rule)

	body := mustMarshal(map[string]any{
		"model":      reqModel,
		"max_tokens": 64,
		"stream":     false,
		"system": []map[string]any{
			{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.86.d9e; cc_entrypoint=sdk-cli; cch=00000; cc_is_subagent=true;"},
			{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
		},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{
				{"type": "text", "text": "<system-reminder>\nToday's date is 2026-09-02.\n</system-reminder>"},
				{"type": "text", "text": "say hi"},
			}},
		},
		"metadata": map[string]any{
			"user_id": `{"device_id":"client-device","account_uuid":"client-account","session_id":"11111111-2222-3333-4444-555555555555","parent_session_id":"99999999-8888-7777-6666-555555555555"}`,
		},
	})
	headers := map[string]string{
		"User-Agent":                    "claude-cli/2.1.86 (external, cli)",
		"anthropic-beta":                "claude-code-20250219,oauth-2025-04-20,per-turn-control-2026-07-01,message-batches-2024-09-24",
		"x-claude-code-agent-id":        "agent-7",
		"x-claude-code-parent-agent-id": "agent-main",
	}

	res, err := env.dispatch(protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, s.Name,
		"/tingly/claude_code/v1/messages", body, headers, false)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.HTTPStatus != 200 {
		t.Fatalf("request failed: status=%d body=%s", res.HTTPStatus, truncate(string(res.RawBody), 300))
	}
	up := env.virtual.LastRequest(EndpointAnthropic)
	if up == nil {
		t.Fatal("no upstream request captured")
	}

	// ── client headers ──────────────────────────────────────────────────
	if got := up.Headers.Get("User-Agent"); got != "claude-cli/2.1.258 (external, cli)" {
		t.Errorf("User-Agent = %q", got)
	}
	if got := up.Headers.Values("Anthropic-Beta"); len(got) != 1 {
		t.Errorf("anthropic-beta must be one header value, got %v", got)
	} else if want := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07,per-turn-control-2026-07-01"; got[0] != want {
		t.Errorf("anthropic-beta =\n  %s\nwant\n  %s", got[0], want)
	}
	if got := up.Headers.Get("X-Stainless-Package-Version"); got != "0.112.1" {
		t.Errorf("X-Stainless-Package-Version = %q", got)
	}
	if got := up.Headers.Get("X-Stainless-Runtime-Version"); got != "v26.3.0" {
		t.Errorf("X-Stainless-Runtime-Version = %q", got)
	}
	if got := up.Headers.Get("X-App"); got != "cli" {
		t.Errorf("x-app = %q", got)
	}
	if got := up.Headers.Get("X-Stainless-Helper-Method"); got != "" {
		t.Errorf("x-stainless-helper-method must not be sent, got %q", got)
	}
	if got := up.Headers.Get("X-Claude-Code-Session-Id"); got != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("X-Claude-Code-Session-Id = %q", got)
	}
	if got := up.Headers.Get("X-Claude-Code-Agent-Id"); got != "agent-7" {
		t.Errorf("x-claude-code-agent-id = %q", got)
	}
	if got := up.Headers.Get("X-Claude-Code-Parent-Agent-Id"); got != "agent-main" {
		t.Errorf("x-claude-code-parent-agent-id = %q", got)
	}
	if got := up.Headers.Get("Authorization"); got != "Bearer sk-ant-oat01-virtual" {
		t.Errorf("Authorization = %q", got)
	}

	// ── system header + metadata ────────────────────────────────────────
	var upBody struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(up.Body, &upBody); err != nil {
		t.Fatalf("unmarshal upstream body: %v\n%s", err, truncate(string(up.Body), 500))
	}
	if len(upBody.System) != 2 {
		t.Fatalf("system blocks = %d, want 2 (billing header rebuilt in place): %+v", len(upBody.System), upBody.System)
	}
	if want := "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000; cc_is_subagent=true;"; upBody.System[0].Text != want {
		t.Errorf("system[0] =\n  %s\nwant\n  %s", upBody.System[0].Text, want)
	}
	if !strings.HasPrefix(upBody.System[1].Text, "You are Claude Code") {
		t.Errorf("system[1] preamble lost: %q", upBody.System[1].Text)
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(upBody.Metadata.UserID), &meta); err != nil {
		t.Fatalf("metadata.user_id not JSON: %q", upBody.Metadata.UserID)
	}
	if meta["device_id"] == "client-device" || meta["device_id"] == "" {
		t.Errorf("device_id must be rewritten to the gateway's device, got %q", meta["device_id"])
	}
	if meta["session_id"] != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("session_id = %q", meta["session_id"])
	}
	if meta["parent_session_id"] != "99999999-8888-7777-6666-555555555555" {
		t.Errorf("parent_session_id not preserved: %q", upBody.Metadata.UserID)
	}
}
