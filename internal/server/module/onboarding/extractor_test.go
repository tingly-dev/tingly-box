package onboarding

import (
	"context"
	"strings"
	"testing"
)

func newExt() *RuleExtractor { return NewRuleExtractor() }

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func tokenValues(in []TokenCandidate) []string {
	out := make([]string, len(in))
	for i, t := range in {
		out[i] = t.Value
	}
	return out
}

func TestExtractor_EnvFile(t *testing.T) {
	input := `# .env
OPENAI_API_KEY=sk-proj-abcdef1234567890abcdef
OPENAI_BASE_URL=https://api.openai.com/v1
`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(d.URLs, "https://api.openai.com/v1") {
		t.Fatalf("expected URL https://api.openai.com/v1, got %v", d.URLs)
	}
	if !contains(tokenValues(d.Tokens), "sk-proj-abcdef1234567890abcdef") {
		t.Fatalf("expected sk-proj token in %v", tokenValues(d.Tokens))
	}
	// OPENAI_BASE_URL should not show up as a token.
	for _, tok := range d.Tokens {
		if tok.Source == "env:OPENAI_BASE_URL" {
			t.Fatalf("OPENAI_BASE_URL leaked into tokens: %v", tok)
		}
	}
}

func TestExtractor_AnthropicCurl(t *testing.T) {
	input := `curl https://api.anthropic.com/v1/messages \
  -H "x-api-key: sk-ant-api03-XYZxyz0123456789ABCDEF" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json"`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(d.URLs, "https://api.anthropic.com/v1/messages") {
		t.Fatalf("expected anthropic URL, got %v", d.URLs)
	}
	found := false
	for _, tok := range d.Tokens {
		if tok.Value == "sk-ant-api03-XYZxyz0123456789ABCDEF" && tok.Source == "x-api-key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected x-api-key token, got %v", d.Tokens)
	}
}

func TestExtractor_BearerHeader(t *testing.T) {
	input := `curl https://openrouter.ai/api/v1/chat/completions \
  -H "Authorization: Bearer sk-or-v1-abcdef0123456789abcdef"`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var bearerSrc string
	for _, tok := range d.Tokens {
		if tok.Value == "sk-or-v1-abcdef0123456789abcdef" {
			bearerSrc = tok.Source
		}
	}
	// The same value is matched by both bearer and key_prefix; we keep the
	// more informative source.
	if bearerSrc != "bearer" {
		t.Fatalf("expected source=bearer, got %q (tokens=%v)", bearerSrc, d.Tokens)
	}
}

func TestExtractor_PlainKey(t *testing.T) {
	input := `here is my key: sk-ant-api03-justakeynothingelse123456`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d.Tokens) == 0 {
		t.Fatalf("expected at least one token, got none")
	}
	// source is now "token_like" after simplification
	if !strings.HasPrefix(d.Tokens[0].Source, "token") {
		t.Fatalf("expected token_like source, got %q", d.Tokens[0].Source)
	}
	if !strings.HasPrefix(d.Tokens[0].Preview, "sk-a") {
		t.Fatalf("expected masked preview, got %q", d.Tokens[0].Preview)
	}
}

func TestExtractor_OnlyURL(t *testing.T) {
	input := `Try https://api.deepseek.com/v1/chat/completions in your client.`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(d.URLs, "https://api.deepseek.com/v1/chat/completions") {
		t.Fatalf("expected deepseek URL, got %v", d.URLs)
	}
	if len(d.Tokens) != 0 {
		t.Fatalf("expected no tokens, got %v", d.Tokens)
	}
}

func TestExtractor_JSONFields(t *testing.T) {
	input := `{
  "api_key": "sk-someservice-abcdef1234567890",
  "base_url": "https://example.com/api/v1"
}`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(d.URLs, "https://example.com/api/v1") {
		t.Fatalf("expected base_url, got %v", d.URLs)
	}
	if !contains(tokenValues(d.Tokens), "sk-someservice-abcdef1234567890") {
		t.Fatalf("expected api_key token, got %v", tokenValues(d.Tokens))
	}
}

func TestExtractor_MultipleURLsAndTokensDeduped(t *testing.T) {
	input := `https://api.openai.com/v1
https://api.openai.com/v1
sk-proj-abcdef1234567890ABCDEF
sk-proj-abcdef1234567890ABCDEF`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	urlCount := 0
	for _, u := range d.URLs {
		if u == "https://api.openai.com/v1" {
			urlCount++
		}
	}
	if urlCount != 1 {
		t.Fatalf("expected URL deduped to 1, got %d (%v)", urlCount, d.URLs)
	}
	tokCount := 0
	for _, tok := range d.Tokens {
		if tok.Value == "sk-proj-abcdef1234567890ABCDEF" {
			tokCount++
		}
	}
	if tokCount != 1 {
		t.Fatalf("expected token deduped to 1, got %d (%v)", tokCount, d.Tokens)
	}
}

func TestExtractor_PreservesURLPath(t *testing.T) {
	input := `Endpoint: https://example.com/foo/bar?x=1.`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://example.com/foo/bar?x=1"
	if !contains(d.URLs, want) {
		t.Fatalf("expected %q (trailing punct stripped), got %v", want, d.URLs)
	}
}

func TestExtractor_EmptyInput(t *testing.T) {
	d, err := newExt().Extract(context.Background(), "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d.URLs) != 0 || len(d.Tokens) != 0 {
		t.Fatalf("expected empty result, got %+v", d)
	}
}

func TestExtractor_IgnoresShortEnvValues(t *testing.T) {
	input := `LANG=en_US.UTF-8
HOME=/root
PATH=/usr/bin
OPENAI_API_KEY=sk-proj-abcdef1234567890abcdef`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tok := range d.Tokens {
		switch tok.Source {
		case "env:LANG", "env:HOME", "env:PATH":
			t.Fatalf("unrelated env var leaked in as token: %v", tok)
		}
	}
	// And the real key still made it through.
	if !contains(tokenValues(d.Tokens), "sk-proj-abcdef1234567890abcdef") {
		t.Fatalf("expected OPENAI_API_KEY value in tokens, got %v", tokenValues(d.Tokens))
	}
}

func TestExtractor_UUIDToken(t *testing.T) {
	// Test UUID not in env var (env has higher priority, so it would override source)
	input := `Here is my token: 550e8400-e29b-41d4-a716-446655440000`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(tokenValues(d.Tokens), "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("expected UUID token, got %v", tokenValues(d.Tokens))
	}
	// source is now "token_like" after simplification
	found := false
	for _, tok := range d.Tokens {
		if tok.Value == "550e8400-e29b-41d4-a716-446655440000" && tok.Source == "token_like" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected source=token_like for UUID token, got %v", d.Tokens)
	}
}

func TestExtractor_UUIDInEnvVar(t *testing.T) {
	// When UUID is in env var, env source takes priority (higher priority)
	input := `API_TOKEN=550e8400-e29b-41d4-a716-446655440000`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(tokenValues(d.Tokens), "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("expected UUID token, got %v", tokenValues(d.Tokens))
	}
	// Should have env:API_TOKEN source (higher priority than uuid)
	found := false
	for _, tok := range d.Tokens {
		if tok.Value == "550e8400-e29b-41d4-a716-446655440000" && tok.Source == "env:API_TOKEN" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected source=env:API_TOKEN for UUID in env var, got %v", d.Tokens)
	}
}

func TestExtractor_UUIDNoHyphen(t *testing.T) {
	// UUID without hyphens: 32 hex chars
	input := `token: 550e8400e29b41d4a716446655440000`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(tokenValues(d.Tokens), "550e8400e29b41d4a716446655440000") {
		t.Fatalf("expected UUID without hyphens, got %v", tokenValues(d.Tokens))
	}
}

func TestExtractor_GenericLongToken(t *testing.T) {
	// Generic long alphanumeric token that doesn't match specific patterns
	input := `Authorization: abc123XYZdef456GHI789jklMNO012`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be caught by bearer regex first
	if !contains(tokenValues(d.Tokens), "abc123XYZdef456GHI789jklMNO012") {
		t.Fatalf("expected generic token, got %v", tokenValues(d.Tokens))
	}
}

func TestExtractor_ShortPrefixToken(t *testing.T) {
	// Short vendor prefix tokens like 火山 engine's ant-6e2a70f0
	input := `{"token":"ant-6e2a70f0"}`
	d, err := newExt().Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(tokenValues(d.Tokens), "ant-6e2a70f0") {
		t.Fatalf("expected short_prefix token, got %v", tokenValues(d.Tokens))
	}
	// source is now "token_like" after simplification
	found := false
	for _, tok := range d.Tokens {
		if tok.Value == "ant-6e2a70f0" && tok.Source == "token_like" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected source=token_like, got %v", d.Tokens)
	}
}
