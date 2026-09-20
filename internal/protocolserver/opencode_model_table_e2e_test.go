package protocolserver

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestE2E_OpenCodeModelTable_MatchesLiveUpstream proves the static table
// (data.ModelInfo.OpenAIEndpoints in providers.json) still agrees with the real
// OpenCode Zen upstream: a "responses" entry (gpt-5.6-luna) rejects Chat and
// answers on Responses, and a model with no entry (kimi-k3) is plain Chat.
// The table is hand-maintained and only as good as the day it was measured
// (2026-09-08) — this is what notices before a user's bug report does.
//
// Prerequisites: OPENCODE_API_KEY, and OPENCODE_PROXY_URL where the sandbox
// only reaches the internet through a proxy (the transport pool never
// inherits HTTP(S)_PROXY — see NewOpenAIClient).
//
// Run with: go test -v ./internal/protocolserver -run TestE2E_OpenCodeModelTable
func TestE2E_OpenCodeModelTable_MatchesLiveUpstream(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set, skipping e2e test")
	}
	// 2026-09-17: last run hit 401 CreditsError (account balance) on both
	// cases, but each error came from the endpoint the table predicted
	// (.../responses for luna, .../chat/completions for kimi-k3) — a
	// wrong-endpoint failure looks different ("not supported for format ...",
	// or a bare 500). Full 200-OK confirmation is from 2026-09-08.

	tm := catalog.NewProviderCatalogManager(catalog.WithGitHubURL(""))
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	provider := &typ.Provider{
		UUID:     "opencode-e2e-model-table",
		Name:     "OpenCode Go",
		AuthType: ai.AuthTypeAPIKey,
		Token:    apiKey,
		APIBase:  "https://opencode.ai/zen/go/v1",
		ProxyURL: os.Getenv("OPENCODE_PROXY_URL"),
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
	}
	pool := client.NewClientPool()
	ctx := context.Background()

	tests := []struct {
		model        string
		wantOverride ai.OpenAIEndpointMode
	}{
		{"gpt-5.6-luna", ai.EndpointModeResponses},
		{"kimi-k3", ai.EndpointModeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			override := tm.GetOpenAIEndpointOverrideForModel(provider, tt.model)
			if override != tt.wantOverride {
				t.Fatalf("table says %q, want %q", override, tt.wantOverride)
			}

			oc := pool.GetOpenAIClient(ctx, provider, tt.model)
			if oc == nil {
				t.Fatal("no client for provider")
			}

			if override == ai.EndpointModeResponses {
				resp, err := oc.ResponsesNew(ctx, responses.ResponseNewParams{
					Model:           responses.ResponsesModel(tt.model),
					MaxOutputTokens: openai.Int(8),
					Input: responses.ResponseNewParamsInputUnion{
						OfString: openai.String("say ok"),
					},
				})
				if err != nil {
					t.Fatalf("Responses call for a table-marked model failed — the table is stale: %v", err)
				}
				t.Logf("responses: id=%s model=%s", resp.ID, resp.Model)
			} else {
				resp, err := oc.ChatCompletionsNew(ctx, openai.ChatCompletionNewParams{
					Model:     openai.ChatModel(tt.model),
					MaxTokens: openai.Int(8),
					Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("say ok")},
				})
				if err != nil {
					t.Fatalf("Chat call for a model the table leaves untouched failed — it may need a table entry now: %v", err)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("no choices in chat response")
				}
				t.Logf("chat: id=%s model=%s", resp.ID, resp.Model)
			}
		})
	}
}
