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
	scenario := vs.detectScenario(r)

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
	vs.handleOpenAIChat(w, r)
}

func (vs *VirtualServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(r)

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

	streaming := strings.Contains(r.URL.Path, "streamGenerateContent")
	scenario := vs.detectScenario(r)

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

func (vs *VirtualServer) detectScenario(_ *http.Request) string {
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


