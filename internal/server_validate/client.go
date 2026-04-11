package server_validate

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

// ParsedResponse is the result of a request sent to a virtual server.
// It wraps sse.ParsedResult with HTTP-layer fields.
type ParsedResponse struct {
	HTTPStatus   int
	IsStreaming  bool
	StreamEvents []string
	RawBody      []byte

	// Parsed semantics (populated from RawBody or StreamEvents)
	sse.ParsedResult
}

// VirtualClient sends provider-native HTTP requests for testing.
// It can operate standalone (pointed at any URL) or bound to a VirtualServer
// (which auto-registers scenarios before each request).
type VirtualClient struct {
	baseURL    string
	httpClient *http.Client
	server     *VirtualServer // optional; set via WithServer
}

// NewVirtualClient creates a client pointing at baseURL.
func NewVirtualClient(baseURL string) *VirtualClient {
	return &VirtualClient{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
	}
}

// WithServer binds the client to a VirtualServer.
// When bound, Send* methods auto-register the scenario before firing the request.
func (vc *VirtualClient) WithServer(vs *VirtualServer) *VirtualClient {
	vc.server = vs
	return vc
}

// Client returns a VirtualClient pre-pointed at this VirtualServer and bound to it.
func (vs *VirtualServer) Client() *VirtualClient {
	return NewVirtualClient(vs.URL()).WithServer(vs)
}

// ─── Send helpers ──────────────────────────────────────────────────────────────

// SendOpenAIChat sends a request to the OpenAI Chat Completions endpoint.
func (vc *VirtualClient) SendOpenAIChat(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":   streaming,
	}
	return vc.doRequest(t, "POST", vc.baseURL+"/v1/chat/completions", body, streaming, StyleOpenAI)
}

// SendOpenAIResponses sends a request to the OpenAI Responses API endpoint.
func (vc *VirtualClient) SendOpenAIResponses(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":  "gpt-4o",
		"input":  "What is the capital of France?",
		"stream": streaming,
	}
	return vc.doRequest(t, "POST", vc.baseURL+"/v1/responses", body, streaming, StyleOpenAI)
}

// SendAnthropicV1 sends a request to the Anthropic Messages endpoint.
func (vc *VirtualClient) SendAnthropicV1(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":     streaming,
	}
	return vc.doRequest(t, "POST", vc.baseURL+"/v1/messages", body, streaming, StyleAnthropic)
}

// SendGoogle sends a request to the Google GenerateContent endpoint.
func (vc *VirtualClient) SendGoogle(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"role": "user", "parts": []map[string]string{{"text": "What is the capital of France?"}}},
		},
	}
	suffix := "generateContent"
	if streaming {
		suffix = "streamGenerateContent"
	}
	return vc.doRequest(t, "POST", vc.baseURL+"/v1beta/models/gemini-2.0-flash/"+suffix, body, streaming, StyleGoogle)
}

// ─── Internals ─────────────────────────────────────────────────────────────────

// maybeRegister registers the scenario on the bound server, if any.
func (vc *VirtualClient) maybeRegister(s Scenario) {
	if vc.server != nil {
		vc.server.RegisterScenario(s)
	}
}

// doRequest sends an HTTP request and returns a ParsedResponse.
func (vc *VirtualClient) doRequest(t *testing.T, method, url string, body interface{}, streaming bool, style APIStyle) *ParsedResponse {
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

	resp, err := vc.httpClient.Do(req)
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
		result.ParsedResult = parsedResultFromStream(result.StreamEvents, style)
	} else {
		result.RawBody, _ = io.ReadAll(resp.Body)
		result.ParsedResult = parsedResultFromJSON(result.RawBody, style)
	}

	return result
}

func parsedResultFromJSON(raw []byte, style APIStyle) sse.ParsedResult {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return sse.ParsedResult{}
	}
	var r *sse.ParsedResult
	switch style {
	case StyleOpenAI:
		if _, hasOutput := m["output"]; hasOutput {
			r = sse.ParseOpenAIResponsesResult(m)
		} else {
			r = sse.ParseOpenAIChatResult(m)
		}
	case StyleAnthropic:
		r = sse.ParseAnthropicResult(m)
	case StyleGoogle:
		r = sse.ParseGoogleResult(m)
	default:
		return sse.ParsedResult{}
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}

func parsedResultFromStream(events []string, style APIStyle) sse.ParsedResult {
	var r *sse.ParsedResult
	switch style {
	case StyleOpenAI:
		r = sse.AssembleOpenAIStream(events)
	case StyleAnthropic:
		r = sse.AssembleAnthropicStream(events)
	case StyleGoogle:
		r = sse.AssembleGoogleStream(events)
	default:
		return sse.ParsedResult{}
	}
	if r == nil {
		return sse.ParsedResult{}
	}
	return *r
}
