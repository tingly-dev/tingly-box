package main

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// An oauth_token stands in for apikey; an unexpanded oauth_token ref is
// reported as the field the user wrote, not as a missing apikey.
func TestMissingFieldsOAuthToken(t *testing.T) {
	base := protocoltest.RealModelEntry{
		BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-5", APIStyle: "anthropic",
	}
	cases := []struct {
		name  string
		key   string
		token string
		want  []string
	}{
		{"token only", "", "sk-ant-oat01-x", nil},
		{"token and key", "sk-ant-api", "sk-ant-oat01-x", nil},
		{"unexpanded token", "", "${CLAUDE_CODE_OAUTH_TOKEN}", []string{"oauth_token"}},
		{"unexpanded token with key", "sk-ant-api", "$CLAUDE_CODE_OAUTH_TOKEN", []string{"oauth_token"}},
		{"neither", "", "", []string{"apikey"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := base
			e.APIKey = c.key
			e.OAuthToken = c.token
			got := missingFields(e)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("missingFields = %v, want %v", got, c.want)
			}
		})
	}
}

// init-config keeps skipping OAuth-only templates except Claude Code, which
// is emitted with an oauth_token env placeholder instead of an empty apikey.
func TestBuildProvidersConfigEmitsClaudeCodeOAuth(t *testing.T) {
	templates := map[string]*catalog.ProviderCatalog{
		"claude-code": {ID: "claude-code", AuthType: "oauth", BaseURLAnthropic: "https://api.anthropic.com",
			Models: []catalog.ModelInfo{{ID: "claude-sonnet-4-5"}}},
		"codex": {ID: "codex", AuthType: "oauth", BaseURLOpenAI: "https://chatgpt.com/backend-api/codex",
			Models: []catalog.ModelInfo{{ID: "gpt-5"}}},
		"anthropic": {ID: "anthropic", AuthType: "key", BaseURLAnthropic: "https://api.anthropic.com",
			Models: []catalog.ModelInfo{{ID: "claude-sonnet-4-5"}}},
	}
	out := buildProvidersConfig(templates)

	if !strings.Contains(out, `- name: "claude-code"`) {
		t.Fatalf("claude-code entry missing:\n%s", out)
	}
	if !strings.Contains(out, `oauth_token: "${CLAUDE_CODE_OAUTH_TOKEN}"`) {
		t.Errorf("claude-code entry must carry the oauth_token placeholder:\n%s", out)
	}
	if strings.Contains(out, `- name: "codex"`) {
		t.Errorf("other OAuth-only templates must still be skipped:\n%s", out)
	}
	// The api-key entry keeps its shape, and the OAuth entry has no apikey line.
	ccBlock := out[strings.Index(out, `- name: "claude-code"`):]
	if i := strings.Index(ccBlock, "\n\n"); i > 0 {
		ccBlock = ccBlock[:i]
	}
	if strings.Contains(ccBlock, "apikey:") {
		t.Errorf("claude-code entry must not emit apikey:\n%s", ccBlock)
	}
	if !strings.Contains(out, `- name: "anthropic"`) || strings.Count(out, `apikey: ""`) != 1 {
		t.Errorf("api-key entry shape changed:\n%s", out)
	}
}
