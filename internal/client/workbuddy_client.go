package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guard
var _ OpenAIClientInterface = (*WorkBuddyClient)(nil)

// WorkBuddyClient wraps OpenAIClient for WorkBuddy OAuth providers. The
// upstream is OpenAI chat completions at {origin}/v2/chat/completions, so the
// wrapper adds only the client identification headers (workbuddyRoundTripper)
// and the model list, which WorkBuddy serves from /v3/config rather than
// /models. See .design/workbuddy-oauth.md.
type WorkBuddyClient struct {
	*OpenAIClient
	httpClient *http.Client
	origin     string
}

// NewWorkBuddyClient creates a WorkBuddy client wrapper.
func NewWorkBuddyClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*WorkBuddyClient, error) {
	if provider.OAuthDetail == nil {
		return nil, fmt.Errorf("workbuddy client requires OAuth configuration")
	}
	if !ai.IsWorkBuddyIssuer(provider.OAuthDetail.GetIssuer()) {
		return nil, fmt.Errorf("workbuddy client can only work for WorkBuddy providers")
	}

	transport := newWorkBuddyRoundTripper(createSessionBoundTransport(provider, sessionID), provider)
	httpClient := &http.Client{
		Transport: wrapWithLogging(transport, provider),
	}

	base, err := NewOpenAIClient(provider, model, sessionID, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create base OpenAI client: %w", err)
	}

	return &WorkBuddyClient{
		OpenAIClient: base,
		httpClient:   httpClient,
		origin:       strings.TrimSuffix(strings.TrimSuffix(provider.APIBase, "/"), "/v2"),
	}, nil
}

// ResponsesNew is not served by WorkBuddy; Responses requests are converted to
// chat completions upstream of the client (endpoint mode chat).
func (c *WorkBuddyClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return nil, fmt.Errorf("workbuddy: /responses endpoint is not supported")
}

// ResponsesNewStreaming is not served by WorkBuddy.
func (c *WorkBuddyClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return nil
}

// ListModels reads the account's model list from GET {origin}/v3/config.
func (c *WorkBuddyClient) ListModels(ctx context.Context) (*ModelListResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+"/v3/config", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.GetProvider().GetAccessToken())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("workbuddy models: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("workbuddy models: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return &ModelListResult{Raw: json.RawMessage(raw)}, fmt.Errorf("workbuddy models: status %d", resp.StatusCode)
	}
	models, err := parseWorkBuddyModels(raw)
	if err != nil {
		return &ModelListResult{Raw: json.RawMessage(raw)}, err
	}
	return &ModelListResult{Models: models, Raw: json.RawMessage(raw)}, nil
}

// parseWorkBuddyModels extracts model ids from a /v3/config body:
// {code, data: {models: [{id|name|model, ...}]}}. A non-zero code is an error
// carrying the server's message.
func parseWorkBuddyModels(raw []byte) ([]string, error) {
	var env struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Models []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Model string `json:"model"`
			} `json:"models"`
			// Agents name the models each client agent may use; the CLI
			// agent's are the ones chats are served for.
			Agents []struct {
				Name   string   `json:"name"`
				Models []string `json:"models"`
			} `json:"agents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("workbuddy models: decode: %w", err)
	}
	if env.Code != nil && *env.Code != 0 {
		return nil, fmt.Errorf("workbuddy models: code=%d msg=%s", *env.Code, env.Msg)
	}
	for _, agent := range env.Data.Agents {
		if agent.Name == "cli" && len(agent.Models) > 0 {
			return dedupeNonEmpty(agent.Models), nil
		}
	}
	seen := make(map[string]bool, len(env.Data.Models))
	models := make([]string, 0, len(env.Data.Models))
	for _, m := range env.Data.Models {
		id := m.ID
		if id == "" {
			id = m.Model
		}
		if id == "" {
			id = m.Name
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	return models, nil
}

func dedupeNonEmpty(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
