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
	if spec.ProviderURL != "https://openrouter.ai/api/v1" {
		t.Errorf("provider URL: got %v", spec.ProviderURL)
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
		"--agent", "--provider-url", "--provider-token", "--provider-style", "--model",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	// --provider-name must not appear — it no longer exists
	if strings.Contains(msg, "--provider-name") {
		t.Errorf("error should not mention --provider-name (removed flag)")
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
// the second call with the same URL must update in place rather than create
// a duplicate provider row.
func TestCI_UpsertProvider_CreateThenUpdate(t *testing.T) {
	am := newTestAppManager(t)

	first := &ciSpec{
		ProviderURL:   "https://example.com/v1",
		ProviderToken: "token-A",
		ProviderStyle: protocol.APIStyleOpenAI,
	}
	uuid1, action, err := upsertProviderByURL(am, first)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if action != "created" {
		t.Errorf("first action: got %q want %q", action, "created")
	}

	// Second call: same URL, different token+style. Must update, not create.
	second := &ciSpec{
		ProviderURL:   "https://example.com/v1",
		ProviderToken: "token-B",
		ProviderStyle: protocol.APIStyleAnthropic,
	}
	uuid2, action, err := upsertProviderByURL(am, second)
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
	got, err := am.GetProvider(uuid1)
	if err != nil || got == nil {
		t.Fatalf("provider lookup after upsert failed: %v", err)
	}
	if got.Token != "token-B" {
		t.Errorf("Token not updated: got %q", got.Token)
	}
	if got.APIStyle != protocol.APIStyleAnthropic {
		t.Errorf("APIStyle not updated: got %q", got.APIStyle)
	}

	// Still exactly one provider.
	if got := len(am.ListProviders()); got != 1 {
		t.Errorf("expected 1 provider after upsert, got %d", got)
	}
}

// TestCI_UpsertProvider_DifferentURLsCreateSeparate ensures two different
// URLs produce two distinct provider rows (no cross-contamination).
func TestCI_UpsertProvider_DifferentURLsCreateSeparate(t *testing.T) {
	am := newTestAppManager(t)

	for _, url := range []string{"https://a.example.com/v1", "https://b.example.com/v1"} {
		_, action, err := upsertProviderByURL(am, &ciSpec{
			ProviderURL:   url,
			ProviderToken: "tok",
			ProviderStyle: protocol.APIStyleOpenAI,
		})
		if err != nil {
			t.Fatalf("upsert %s: %v", url, err)
		}
		if action != "created" {
			t.Errorf("upsert %s: got action %q want created", url, action)
		}
	}
	if got := len(am.ListProviders()); got != 2 {
		t.Errorf("expected 2 providers, got %d", got)
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
