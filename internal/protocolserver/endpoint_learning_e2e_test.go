package protocolserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestE2E_OpenCodePerModelEndpoints drives the real OpenCode Zen upstream to
// prove the two facts the per_model mode is built on, rather than trusting a
// stand-in for them:
//
//   - a Responses-only model (gpt-5.6-luna) really is rejected on
//     /chat/completions and really answers on /responses;
//   - a Chat-only model (kimi-k3) is the mirror image.
//
// Prerequisites: OPENCODE_API_KEY, and OPENCODE_PROXY_URL where the network
// only leaves through a proxy (the transport pool never inherits HTTP(S)_PROXY).
//
// Run with: go test -v ./internal/protocolserver -run TestE2E_OpenCodePerModel
func TestE2E_OpenCodePerModelEndpoints(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set, skipping e2e test")
	}

	provider := &typ.Provider{
		UUID:               "opencode-e2e-endpoint",
		Name:               "OpenCode Go",
		AuthType:           ai.AuthTypeAPIKey,
		Token:              apiKey,
		APIBase:            "https://opencode.ai/zen/go/v1",
		ProxyURL:           os.Getenv("OPENCODE_PROXY_URL"),
		APIStyle:           protocol.APIStyleOpenAI,
		OpenAIEndpointMode: ai.EndpointModePerModel,
		Enabled:            true,
	}

	tests := []struct {
		model     string
		chatOK    bool
		responses bool
	}{
		{model: "gpt-5.6-luna", chatOK: false, responses: true},
		{model: "kimi-k3", chatOK: true, responses: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			gotChat, chatBody := postChat(t, provider, tt.model)
			gotResponses, responsesBody := postResponses(t, provider, tt.model)
			t.Logf("%s: chat=%d responses=%d", tt.model, gotChat, gotResponses)

			if (gotChat == http.StatusOK) != tt.chatOK {
				t.Errorf("chat status %d, want ok=%v", gotChat, tt.chatOK)
			}
			if (gotResponses == http.StatusOK) != tt.responses {
				t.Errorf("responses status %d, want ok=%v", gotResponses, tt.responses)
			}
			// The rejection must be one the learning path recognizes, or the
			// retry would never fire for this model.
			if !tt.chatOK && !looksLikeEndpointMismatch(gotChat, chatBody) {
				t.Errorf("chat rejection %d/%s is not recognized as an endpoint mismatch", gotChat, chatBody)
			}
			if !tt.responses && !looksLikeEndpointMismatch(gotResponses, responsesBody) {
				t.Errorf("responses rejection %d/%s is not recognized as an endpoint mismatch", gotResponses, responsesBody)
			}
		})
	}
}

func postChat(t *testing.T, provider *typ.Provider, model string) (int, []byte) {
	t.Helper()
	body := `{"model":"` + model + `","max_tokens":4,"messages":[{"role":"user","content":"hi"}]}`
	return postUpstream(t, provider, "/chat/completions", body)
}

func postResponses(t *testing.T, provider *typ.Provider, model string) (int, []byte) {
	t.Helper()
	body := `{"model":"` + model + `","max_output_tokens":16,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`
	return postUpstream(t, provider, "/responses", body)
}

// postUpstream sends one request over the same client transport chain a
// dispatch attempt would use, so the session header and proxy handling match
// the real path.
func postUpstream(t *testing.T, provider *typ.Provider, path, body string) (int, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/openai"+path, strings.NewReader(body))

	oc := client.NewClientPool().GetOpenAIClient(c.Request.Context(), provider, "")
	if oc == nil {
		t.Fatal("no client for provider")
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
		strings.TrimSuffix(provider.APIBase, "/")+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())

	resp, err := oc.(*client.OpenCodeClient).HttpClient.Do(req)
	if err != nil {
		t.Fatalf("upstream: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	raw, _ := json.Marshal(payload)
	return resp.StatusCode, raw
}
