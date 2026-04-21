// Package server_validate provides a mock HTTP provider server that speaks
// OpenAI, Anthropic, and Google response formats for testing purposes.
//
// A VirtualServer acts as a deterministic "virtual model" — scenario responses
// are pre-configured and returned without any real model calls. It is used by
// the virtualmodel test framework to exercise the gateway's protocol transform
// pipeline end-to-end.
//
// Use VirtualClient (client.go) to send requests to the server and inspect
// parsed responses. A bound client is obtained via vs.Client().
package server_validate

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

// APIStyle aliases protocol.APIStyle for scenario keying.
type APIStyle = protocol.APIStyle

const (
	StyleOpenAI    = protocol.APIStyleOpenAI
	StyleAnthropic = protocol.APIStyleAnthropic
	StyleGoogle    = protocol.APIStyleGoogle
)

// MockResponseBuilder defines how a virtual server should respond for one provider style.
type MockResponseBuilder struct {
	// NonStream returns the HTTP status code and response body bytes.
	NonStream func() (statusCode int, body []byte)
	// Stream returns the SSE event lines (each line is "data: ..." or "event: ...").
	Stream func() []string
}

// Scenario is a named test scenario describing what the mock provider returns.
type Scenario struct {
	Name        string
	Description string
	Tags        []string

	// MockResponses keyed by provider APIStyle ("openai", "anthropic", "google").
	MockResponses map[APIStyle]MockResponseBuilder
}

// VirtualServer is a mock provider server backed by httptest.Server.
// It speaks OpenAI, Anthropic, and Google response formats and returns
// pre-configured scenario responses.
type VirtualServer struct {
	server    *httptest.Server
	scenarios map[string]Scenario // keyed by scenario name

	mu        sync.RWMutex
	callCount int
}

// NewVirtualServer creates a new VirtualServer and registers cleanup with t.
func NewVirtualServer(t *testing.T) *VirtualServer {
	t.Helper()
	vs := &VirtualServer{
		scenarios: make(map[string]Scenario),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)
	mux.HandleFunc("/", vs.handleGoogle) // catches /v1beta/models/*/generateContent

	vs.server = httptest.NewServer(mux)
	t.Cleanup(vs.Close)
	return vs
}

// NewVirtualServerForCLI creates a new VirtualServer for CLI use (without testing.T).
// The caller must call Close() to clean up resources.
func NewVirtualServerForCLI() *VirtualServer {
	vs := &VirtualServer{
		scenarios: make(map[string]Scenario),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)
	mux.HandleFunc("/messages", vs.handleAnthropicMessages)
	mux.HandleFunc("/", vs.handleGoogle) // catches /v1beta/models/*/generateContent

	vs.server = httptest.NewServer(mux)
	return vs
}

// RegisterScenario registers a scenario so the virtual server can serve its mock responses.
func (vs *VirtualServer) RegisterScenario(s Scenario) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.scenarios[s.Name] = s
}

// URL returns the base URL of the virtual server.
func (vs *VirtualServer) URL() string {
	return vs.server.URL
}

// Close shuts down the virtual server.
func (vs *VirtualServer) Close() {
	vs.server.Close()
}

// CallCount returns the total number of requests received.
func (vs *VirtualServer) CallCount() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.callCount
}

// ─── HTTP handlers ─────────────────────────────────────────────────────────────

func (vs *VirtualServer) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleOpenAI]
	if !ok {
		http.Error(w, "no openai mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		sse.WriteSSEResponse(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

func (vs *VirtualServer) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)

	if streaming {
		// Return Responses API SSE streaming format
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		lines := []string{
			`data: {"type":"response.created","response":{"id":"resp_vs_001","object":"realtime.response","model":"virtual","status":"in_progress","output":[],"usage":null}}`,
			``,
			`data: {"type":"response.output_item.added","response_id":"resp_vs_001","item":{"id":"item_vs_001","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
			``,
			`data: {"type":"response.content_part.added","response_id":"resp_vs_001","item_id":"item_vs_001","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
			``,
			`data: {"type":"response.output_text.delta","response_id":"resp_vs_001","item_id":"item_vs_001","output_index":0,"content_index":0,"delta":"Hello, world!"}`,
			``,
			`data: {"type":"response.output_text.done","response_id":"resp_vs_001","item_id":"item_vs_001","output_index":0,"content_index":0,"text":"Hello, world!"}`,
			``,
			`data: {"type":"response.content_part.done","response_id":"resp_vs_001","item_id":"item_vs_001","output_index":0,"content_index":0,"part":{"type":"output_text","text":"Hello, world!"}}`,
			``,
			`data: {"type":"response.output_item.done","response_id":"resp_vs_001","item":{"id":"item_vs_001","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello, world!"}]}}`,
			``,
			`data: {"type":"response.completed","response":{"id":"resp_vs_001","object":"realtime.response","model":"virtual","status":"completed","output":[{"id":"item_vs_001","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello, world!"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
			``,
			`data: [DONE]`,
			``,
		}
		for _, line := range lines {
			w.Write([]byte(line + "\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	} else {
		// Return Responses API non-streaming format
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp_vs_001","object":"realtime.response","model":"virtual","status":"completed","output":[{"id":"item_vs_001","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello, world!"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`))
	}
}

func (vs *VirtualServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleAnthropic]
	if !ok {
		http.Error(w, "no anthropic mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		sse.WriteSSEResponse(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

func (vs *VirtualServer) handleGoogle(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := strings.Contains(r.URL.Path, "streamGenerateContent")
	scenario := vs.detectScenarioFromURLOrBody(r.URL.Path, bodyBytes)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleGoogle]
	if !ok {
		http.Error(w, "no google mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		sse.WriteSSEResponse(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func (vs *VirtualServer) parseStreamFlagFromBytes(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	flag, _ := m["stream"].(bool)
	return flag
}

// detectScenarioFromURLOrBody extracts scenario from Google-style URL path or falls back to body.
// Google requests encode the model in the URL: /v1beta/models/{model}/generateContent
func (vs *VirtualServer) detectScenarioFromURLOrBody(urlPath string, body []byte) string {
	const prefix = "virtual-model-"
	// Try URL path first: extract model segment
	parts := strings.Split(urlPath, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, prefix) {
			name := part[len(prefix):]
			// Strip trailing action suffix if any (e.g. ":generateContent" edge case)
			if idx := strings.IndexByte(name, ':'); idx >= 0 {
				name = name[:idx]
			}
			vs.mu.RLock()
			_, exists := vs.scenarios[name]
			vs.mu.RUnlock()
			if exists {
				return name
			}
		}
	}
	// Fallback to body parsing
	return vs.detectScenario(body)
}

// detectScenario extracts the scenario name from the request body's model field.
// The model field is expected to be "virtual-model-{scenario}" (set by SetupRoute).
// Falls back to the first registered scenario if extraction fails.
func (vs *VirtualServer) detectScenario(body []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err == nil {
		if model, ok := m["model"].(string); ok {
			const prefix = "virtual-model-"
			if strings.HasPrefix(model, prefix) {
				name := model[len(prefix):]
				vs.mu.RLock()
				_, exists := vs.scenarios[name]
				vs.mu.RUnlock()
				if exists {
					return name
				}
			}
		}
	}
	// Fallback: return first registered scenario name
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	for name := range vs.scenarios {
		return name
	}
	return ""
}

func (vs *VirtualServer) firstScenario() Scenario {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	for _, s := range vs.scenarios {
		return s
	}
	return Scenario{}
}
