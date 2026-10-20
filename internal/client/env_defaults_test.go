package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// captureHeaders returns a server that records the headers of the last request.
func captureHeaders(t *testing.T) (*httptest.Server, func() http.Header) {
	t.Helper()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() http.Header { return got }
}

// The gateway process environment (e.g. ANTHROPIC_AUTH_TOKEN exported so
// Claude Code can reach the gateway) must never reach an upstream provider.
func TestAnthropicClient_IgnoresEnvironmentCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "gateway-env-token")
	t.Setenv("ANTHROPIC_API_KEY", "")
	srv, headers := captureHeaders(t)

	provider := &typ.Provider{Name: "p", APIBase: srv.URL, APIStyle: protocol.APIStyleAnthropic, Token: "provider-key"}
	c, err := newAnthropicClientWithTransport(provider, srv.URL, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Client().Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     "m",
		MaxTokens: 1,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	})

	h := headers()
	if got := h.Get("X-Api-Key"); got != "provider-key" {
		t.Errorf("X-Api-Key = %q, want provider-key", got)
	}
	if got := h.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q leaked from environment, want empty", got)
	}
}

func TestOpenAIClient_IgnoresEnvironmentConfig(t *testing.T) {
	t.Setenv("OPENAI_ORG_ID", "org-env")
	t.Setenv("OPENAI_PROJECT_ID", "proj-env")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Env-Leak: 1")
	srv, headers := captureHeaders(t)

	provider := &typ.Provider{Name: "p", APIBase: srv.URL, APIStyle: protocol.APIStyleOpenAI, Token: "provider-key"}
	c, err := newOpenAIClientWithTransport(provider, srv.URL, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Client().Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model:    "m",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
	})

	h := headers()
	if got := h.Get("Authorization"); got != "Bearer provider-key" {
		t.Errorf("Authorization = %q, want provider key", got)
	}
	for _, k := range []string{"OpenAI-Organization", "OpenAI-Project", "X-Env-Leak"} {
		if got := h.Get(k); got != "" {
			t.Errorf("%s = %q leaked from environment, want empty", k, got)
		}
	}
}
