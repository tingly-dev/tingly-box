package vmodeltest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
)

// ParsedResponse is the result of a request sent to a virtual server.
type ParsedResponse struct {
	HTTPStatus   int
	IsStreaming  bool
	StreamEvents []string
	RawBody      []byte

	sse.ParsedResult
}

// ToRoundTrip adapts a ParsedResponse into a check.RoundTripResult so the
// reusable assertion library (vmodel/benchmark/check) can run against it. This
// is the bridge that lets any consumer — protocoltest, the benchmark, or an
// external Go project — send with this client and assert with check, without
// hand-rolling the conversion.
func (p *ParsedResponse) ToRoundTrip() *check.RoundTripResult {
	r := &check.RoundTripResult{
		IsStreaming:     p.IsStreaming,
		HTTPStatus:      p.HTTPStatus,
		RawBody:         p.RawBody,
		StreamEvents:    p.StreamEvents,
		Content:         p.Content,
		Role:            p.Role,
		Model:           p.Model,
		FinishReason:    p.FinishReason,
		ThinkingContent: p.ThinkingContent,
	}
	for _, tc := range p.ToolCalls {
		r.ToolCalls = append(r.ToolCalls, check.ToolCallResult{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	if p.Usage != nil {
		r.Usage = &check.TokenUsage{InputTokens: p.Usage.InputTokens, OutputTokens: p.Usage.OutputTokens}
	}
	return r
}

// Client sends provider-native HTTP requests for testing vmodel endpoints.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a client pointing at baseURL.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: http.DefaultClient,
	}
}

// SendOpenAIChatModel sends a request to the OpenAI Chat Completions endpoint.
func (c *Client) SendOpenAIChatModel(t *testing.T, modelID string, streaming bool) *ParsedResponse {
	t.Helper()
	body := map[string]interface{}{
		"model":    modelID,
		"messages": []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":   streaming,
	}
	return c.DoRequest(t, "POST", c.BaseURL+"/v1/chat/completions", body, streaming, protocol.APIStyleOpenAI)
}

// SendAnthropicV1Model sends a request to the Anthropic Messages endpoint.
func (c *Client) SendAnthropicV1Model(t *testing.T, modelID string, streaming bool) *ParsedResponse {
	t.Helper()
	body := map[string]interface{}{
		"model":      modelID,
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":     streaming,
	}
	return c.DoRequest(t, "POST", c.BaseURL+"/v1/messages", body, streaming, protocol.APIStyleAnthropic)
}

// SendAnthropicBetaModel sends a request to the Anthropic Messages endpoint
// with the ?beta=true query flag.
func (c *Client) SendAnthropicBetaModel(t *testing.T, modelID string, streaming bool) *ParsedResponse {
	t.Helper()
	body := map[string]interface{}{
		"model":      modelID,
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":     streaming,
	}
	return c.DoRequest(t, "POST", c.BaseURL+"/v1/messages?beta=true", body, streaming, protocol.APIStyleAnthropic)
}

// DoRequest sends an HTTP request and returns a ParsedResponse.
func (c *Client) DoRequest(t *testing.T, method, url string, body interface{}, streaming bool, style protocol.APIStyle) *ParsedResponse {
	t.Helper()

	reqBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(method, url, strings.NewReader(string(reqBody)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	result := &ParsedResponse{
		HTTPStatus:  resp.StatusCode,
		IsStreaming: streaming,
	}

	if streaming {
		result.StreamEvents, result.RawBody = sse.ReadSSELines(resp.Body)
		result.ParsedResult = ParsedResultFromStream(result.StreamEvents, style)
	} else {
		result.RawBody, _ = io.ReadAll(resp.Body)
		result.ParsedResult = ParsedResultFromJSON(result.RawBody, style)
	}

	return result
}

// ParsedResultFromJSON parses a JSON response body based on provider style.
func ParsedResultFromJSON(raw []byte, style protocol.APIStyle) sse.ParsedResult {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return sse.ParsedResult{}
	}
	var r *sse.ParsedResult
	switch style {
	case protocol.APIStyleOpenAI:
		if _, hasOutput := m["output"]; hasOutput {
			r = sse.ParseOpenAIResponsesResult(m)
		} else {
			r = sse.ParseOpenAIChatResult(m)
		}
	case protocol.APIStyleAnthropic:
		r = sse.ParseAnthropicResult(m)
	case protocol.APIStyleGoogle:
		r = sse.ParseGoogleResult(m)
	default:
		return sse.ParsedResult{}
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}

// ParsedResultFromStream assembles a parsed result from SSE events.
func ParsedResultFromStream(events []string, style protocol.APIStyle) sse.ParsedResult {
	var r *sse.ParsedResult
	switch style {
	case protocol.APIStyleOpenAI:
		r = sse.AssembleOpenAIStream(events)
	case protocol.APIStyleAnthropic:
		r = sse.AssembleAnthropicStream(events)
	case protocol.APIStyleGoogle:
		r = sse.AssembleGoogleStream(events)
	default:
		return sse.ParsedResult{}
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}
