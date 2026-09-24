package protocoltest

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// TestSetupRealOAuthAgent_ClaudeCode is the hermetic twin of
// `harness replay claude --upstream real` with an oauth_token entry: the
// "real" provider is the virtual server, so the test can inspect what the
// gateway actually sends. It pins that the OAuth real-provider path
//
//   - authenticates with the bearer token (not an x-api-key),
//   - is signed as the newest native Claude Code client
//     (typ.ClaudeCodeVersionLatest) rather than the legacy emulation,
//   - rebuilds the billing header in place and keeps the Claude Code
//     preamble the fixture carries,
//
// so a green live run means Anthropic accepted exactly this shape.
func TestSetupRealOAuthAgent_ClaudeCode(t *testing.T) {
	env, err := NewAgentTestEnv(AgentTypeClaudeCode)
	if err != nil {
		t.Fatalf("NewAgentTestEnv: %v", err)
	}
	defer env.Close(false)

	const token = "sk-ant-oat01-harness-virtual"
	const accountUUID = "0d6f2c1e-4b6a-4f8e-9a5d-2f7c1b3e8a90"
	if err := env.SetupRealOAuthAgent(AgentTypeClaudeCode, "virtual-as-oauth", "claude-sonnet-4-5", env.VirtualServerURL(), token, accountUUID); err != nil {
		t.Fatalf("SetupRealOAuthAgent: %v", err)
	}

	_, requestModel, err := BuiltinRuleRef(AgentTypeClaudeCode)
	if err != nil {
		t.Fatalf("BuiltinRuleRef: %v", err)
	}
	// Same shape as cli/harness/testdata/fixtures/anthropic/text.json: the
	// Claude Code preamble, no billing header block, a non-JSON metadata
	// user_id (the fixture's legacy form).
	body, _ := json.Marshal(map[string]any{
		"model":      requestModel,
		"max_tokens": 64,
		"stream":     false,
		"system": []map[string]any{
			{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
		},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "text", "text": "say hi"}}},
		},
		"metadata": map[string]any{"user_id": "harness-replay-fixture"},
	})

	res, err := env.ReplayFixture(AgentTypeClaudeCode, body, false)
	if err != nil {
		t.Fatalf("ReplayFixture: %v", err)
	}
	if res.HTTPStatus != 200 {
		t.Fatalf("status = %d, body = %s", res.HTTPStatus, truncate(string(res.RawBody), 300))
	}

	up := env.virtualServer.LastRequest(EndpointAnthropic)
	if up == nil {
		t.Fatal("no upstream request captured")
	}
	if got := up.Headers.Get("Authorization"); got != "Bearer "+token {
		t.Errorf("Authorization = %q, want the OAuth bearer token", got)
	}
	if got := up.Headers.Get("X-Api-Key"); got != "" {
		t.Errorf("x-api-key must not be sent on the OAuth path, got %q", got)
	}
	if got := up.Headers.Get("User-Agent"); got != "claude-cli/2.1.280 (external, cli)" {
		t.Errorf("User-Agent = %q, want the latest native client", got)
	}
	if got := up.Headers.Get("X-Claude-Code-Request-Class"); got != "main" {
		t.Errorf("x-claude-code-request-class = %q, want main", got)
	}
	if got := up.Headers.Values("Anthropic-Beta"); len(got) != 1 || !strings.HasPrefix(got[0], "claude-code-20250219,oauth-2025-04-20,") {
		t.Errorf("anthropic-beta = %v", got)
	}

	var parsed struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(up.Body, &parsed); err != nil {
		t.Fatalf("unmarshal upstream body: %v", err)
	}
	if len(parsed.System) != 2 {
		t.Fatalf("system blocks = %d, want billing header + preamble: %+v", len(parsed.System), parsed.System)
	}
	if want := regexp.MustCompile(`^x-anthropic-billing-header: cc_version=2\.1\.280\.[0-9a-f]{3}; cc_entrypoint=cli; cch=[0-9a-f]{5};$`); !want.MatchString(parsed.System[0].Text) {
		t.Errorf("billing header = %q", parsed.System[0].Text)
	}
	if strings.Contains(parsed.System[0].Text, "cch=00000;") {
		t.Errorf("cch placeholder reached the wire unpatched: %s", parsed.System[0].Text)
	}
	if !strings.HasPrefix(parsed.System[1].Text, "You are Claude Code") {
		t.Errorf("preamble lost: %q", parsed.System[1].Text)
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(parsed.Metadata.UserID), &meta); err != nil {
		t.Fatalf("metadata.user_id is not the native JSON form: %q", parsed.Metadata.UserID)
	}
	if meta["account_uuid"] != accountUUID {
		t.Errorf("metadata account_uuid = %q, want the provider's account uuid %q", meta["account_uuid"], accountUUID)
	}
	if meta["device_id"] == "" || meta["session_id"] == "" {
		t.Errorf("metadata missing device/session: %v", meta)
	}
}

// TestSetupRealOAuthAgent_ClaudeOnly pins that the OAuth path refuses agents
// that have no Claude Code OAuth chain instead of silently sending a bearer
// token to an unrelated provider.
func TestSetupRealOAuthAgent_ClaudeOnly(t *testing.T) {
	env, err := NewAgentTestEnv(AgentTypeCodex)
	if err != nil {
		t.Fatalf("NewAgentTestEnv: %v", err)
	}
	defer env.Close(false)
	if err := env.SetupRealOAuthAgent(AgentTypeCodex, "p", "m", env.VirtualServerURL(), "tok", ""); err == nil {
		t.Fatal("expected an error for a non-claude agent")
	}
	if err := env.SetupRealOAuthAgent(AgentTypeClaudeCode, "p", "m", env.VirtualServerURL(), "", ""); err == nil {
		t.Fatal("expected an error for an empty token")
	}
}
