package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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

	transport := newWorkBuddyRoundTripper(createSessionBoundTransport(provider, sessionID), provider, sessionID)
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

// ChatCompletionsNew answers a complete call from a stream: WorkBuddy refuses
// a chat that is not streamed, so the request is sent with stream: true and
// the chunks are assembled into the one ChatCompletion the caller expects.
// This is the same fallback CodexClient.ResponsesNew makes for the ChatGPT
// backend, kept at the client so every complete-call path (passthrough,
// cross-protocol, stage, tool engine) gets it. A stream that yields no answer
// is an EmptyResponseError, so failover can retry it.
func (c *WorkBuddyClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	// Ask for the closing usage chunk, so the assembled answer carries usage.
	req.StreamOptions.IncludeUsage = openai.Bool(true)
	stream := c.OpenAIClient.ChatCompletionsNewStreaming(ctx, req)
	if stream == nil {
		return nil, fmt.Errorf("workbuddy: chat stream not started")
	}
	defer stream.Close()

	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		if !acc.AddChunk(stream.Current()) {
			return nil, fmt.Errorf("workbuddy: malformed chat stream chunk")
		}
	}
	if err := stream.Err(); err != nil {
		// An upstream refusal keeps its status; only a stream that broke
		// before any answer is an empty response.
		var apiErr *openai.Error
		if !errors.As(err, &apiErr) && acc.ID == "" && len(acc.Choices) == 0 {
			return nil, &protocol.EmptyResponseError{Err: err}
		}
		return nil, err
	}
	if len(acc.Choices) == 0 {
		return nil, &protocol.EmptyResponseError{Err: fmt.Errorf("workbuddy: chat stream ended without an answer")}
	}
	completion := acc.ChatCompletion
	completion.Object = "chat.completion"
	return &completion, nil
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
