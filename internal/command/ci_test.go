package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// helper: minimal valid flag set, mutated by individual tests.
func validApplyFlags() *CIApplyCmdKong {
	return &CIApplyCmdKong{
		Agent:         "cc",
		ProviderName:  "ci-openrouter",
		ProviderURL:   "https://openrouter.ai/api/v1",
		ProviderToken: "sk-test-token-1234567890",
		ProviderStyle: "openai",
		Model:         "anthropic/claude-sonnet-4",
		Unified:       true,
	}
}

func TestCIApply_toSpec_OK(t *testing.T) {
	c := validApplyFlags()
	spec, err := c.toSpec()
	if err != nil {
		t.Fatalf("toSpec: %v", err)
	}
	if spec.AgentType != agent.AgentTypeClaudeCode {
		t.Errorf("agent type: got %v want %v", spec.AgentType, agent.AgentTypeClaudeCode)
	}
	if spec.ProviderStyle != protocol.APIStyleOpenAI {
		t.Errorf("provider style: got %v want %v", spec.ProviderStyle, protocol.APIStyleOpenAI)
	}
}

func TestCIApply_toSpec_MissingFlagsCollected(t *testing.T) {
	c := &CIApplyCmdKong{} // everything empty
	_, err := c.toSpec()
	if err == nil {
		t.Fatal("expected error when all required flags missing")
	}
	msg := err.Error()
	for _, want := range []string{
		"--agent", "--provider-name", "--provider-url",
		"--provider-token", "--provider-style", "--model",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestCIApply_toSpec_InvalidAgent(t *testing.T) {
	c := validApplyFlags()
	c.Agent = "nope"
	if _, err := c.toSpec(); err == nil {
		t.Fatal("expected error for invalid agent")
	}
}

func TestCIApply_toSpec_RejectsOAuthStyle(t *testing.T) {
	// OAuth-style providers (e.g. "anthropic-oauth", "google") are intentionally
	// out of scope for `ci apply` — they require a browser flow.
	for _, bad := range []string{"google", "anthropic-oauth", "oauth", ""} {
		c := validApplyFlags()
		c.ProviderStyle = bad
		_, err := c.toSpec()
		if err == nil {
			t.Errorf("provider-style %q should be rejected", bad)
		}
	}
}

func TestCIApply_toSpec_StyleIsCaseInsensitive(t *testing.T) {
	c := validApplyFlags()
	c.ProviderStyle = "OpenAI"
	spec, err := c.toSpec()
	if err != nil {
		t.Fatalf("toSpec: %v", err)
	}
	if spec.ProviderStyle != protocol.APIStyleOpenAI {
		t.Errorf("expected APIStyleOpenAI, got %v", spec.ProviderStyle)
	}
}

// TestCI_UpsertProvider_CreateThenUpdate exercises the idempotency contract:
// the second call with the same provider name must update in place rather
// than create a duplicate row.
func TestCI_UpsertProvider_CreateThenUpdate(t *testing.T) {
	am := newTestAppManager(t)

	first := &ciSpec{
		ProviderName:  "ci-openrouter",
		ProviderURL:   "https://example.com/v1",
		ProviderToken: "token-A",
		ProviderStyle: protocol.APIStyleOpenAI,
	}
	uuid1, action, err := upsertProviderByName(am, first)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if action != "created" {
		t.Errorf("first action: got %q want %q", action, "created")
	}

	// Second call: same name, different token+url. Must update, not create.
	second := &ciSpec{
		ProviderName:  "ci-openrouter",
		ProviderURL:   "https://example.com/v2",
		ProviderToken: "token-B",
		ProviderStyle: protocol.APIStyleAnthropic,
	}
	uuid2, action, err := upsertProviderByName(am, second)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if action != "updated" {
		t.Errorf("second action: got %q want %q", action, "updated")
	}
	if uuid1 != uuid2 {
		t.Errorf("upsert created a duplicate: uuid1=%s uuid2=%s", uuid1, uuid2)
	}

	// Confirm the in-place update actually took.
	got, err := am.GetProviderByName("ci-openrouter")
	if err != nil || got == nil {
		t.Fatalf("provider lookup after upsert failed: %v", err)
	}
	if got.APIBase != "https://example.com/v2" {
		t.Errorf("APIBase not updated: got %q", got.APIBase)
	}
	if got.Token != "token-B" {
		t.Errorf("Token not updated: got %q", got.Token)
	}
	if got.APIStyle != protocol.APIStyleAnthropic {
		t.Errorf("APIStyle not updated: got %q", got.APIStyle)
	}

	// And there is still exactly one provider.
	if got := len(am.ListProviders()); got != 1 {
		t.Errorf("expected 1 provider after upsert, got %d", got)
	}
}

func TestRedactToken(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "****"},
		{"short", "****"},
		{"12345678", "****"},
		{"sk-abcdefgh1234", "sk-a…1234"},
	}
	for _, c := range cases {
		if got := redactToken(c.in); got != c.want {
			t.Errorf("redactToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
