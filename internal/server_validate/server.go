// Package server_validate provides a mock HTTP provider server that speaks
// OpenAI, Anthropic, and Google response formats for testing purposes.
//
// **Architecture Note**: VirtualServer is a **provider mock**, not a gateway mock.
// It speaks provider-native formats (OpenAI/Anthropic/Google APIs) at provider-native
// routes (/v1/chat/completions, /v1/messages, /v1beta/models/...).
//
// The test harness routing flow is:
//
//	Client → Gateway (/tingly/{scenario}/v1/...) → Protocol Transform → Virtual Server (/v1/...)
//
// A VirtualServer acts as a deterministic "virtual model" — scenario responses
// are pre-configured and returned without any real model calls. It is used by
// the protocol_validate test framework to exercise the gateway's protocol transform
// pipeline end-to-end.
//
// Use VirtualClient (client.go) to send requests directly to the server and inspect
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
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// ResponseFormat represents the format of the mock response for different endpoints.
type ResponseFormat string

const (
	FormatOpenAIChat      ResponseFormat = "openai_chat"      // /v1/chat/completions
	FormatOpenAIResponses ResponseFormat = "openai_responses" // /v1/responses
	FormatAnthropic       ResponseFormat = "anthropic"        // /v1/messages
	FormatGoogle          ResponseFormat = "google"           // /v1beta/models/.../generateContent
)

// MockResponseBuilder defines how a virtual server should respond for one response format.
type MockResponseBuilder struct {
	// NonStream returns the HTTP status code and response body bytes.
	NonStream func() (statusCode int, body []byte)
	// Stream returns the SSE event lines (each line is "data: ..." or "event: ...").
	Stream func() []string
}

// Scenario is a named test scenario describing what the mock provider returns.
//
// Scenario also satisfies vmodel.VirtualModel via stub identity methods
// so that scenario storage can reuse vmodel.GenericRegistry, the same
// thread-safe registry primitive used by the production virtualmodel sub-
// packages. The byte-replay handlers in this file are intentionally NOT
// wired through virtualserver/handler.go — that handler operates on
// structured request/response shapes, while protocol_validate scenarios are
// pre-rendered byte / SSE-line payloads. Sharing the registry primitive is
// the cleanest reuse available without losing wire-format control.
type Scenario struct {
	Name        string
	Description string
	Tags        []string

	// MockResponses keyed by response format (openai_chat, openai_responses, anthropic, google).
	// Each endpoint (/v1/chat/completions, /v1/responses, /v1/messages, /v1beta/models/.../generateContent)
	// returns the corresponding format.
	MockResponses map[ResponseFormat]MockResponseBuilder
}

// Compile-time check: Scenario satisfies vmodel.VirtualModel.
var _ vmodel.VirtualModel = Scenario{}

// GetID returns the scenario name; scenarios are looked up by name.
func (s Scenario) GetID() string { return s.Name }

// GetName returns the scenario name.
func (s Scenario) GetName() string { return s.Name }

// GetDescription returns the scenario description.
func (s Scenario) GetDescription() string { return s.Description }

// GetType is always Static — scenarios serve fixed pre-rendered responses.
func (s Scenario) GetType() vmodel.VirtualModelType {
	return vmodel.VirtualModelTypeStatic
}

// SimulatedDelay is always 0 — protocol-validate scenarios do not simulate latency.
func (s Scenario) SimulatedDelay() time.Duration { return 0 }

// ToModel returns the OpenAI-compatible models-list entry for this scenario.
func (s Scenario) ToModel() vmodel.Model {
	return vmodel.Model{
		ID:      s.Name,
		Object:  "model",
		Created: 0,
		OwnedBy: vmodel.DefaultMockOwnedBy,
	}
}

// VirtualServer is a mock provider server backed by httptest.Server.
// It speaks OpenAI, Anthropic, and Google response formats and returns
// pre-configured scenario responses.
//
// **Provider Routes**: This server handles provider-native routes (/v1/chat/completions,
// /v1/messages, /v1beta/models/...), NOT gateway routes (/tingly/{scenario}/v1/...).
// The gateway transforms requests to provider format before forwarding to this server.
type VirtualServer struct {
	server    *httptest.Server
	scenarios *vmodel.GenericRegistry[Scenario]

	mu        sync.RWMutex
	callCount int
}

// NewVirtualServer creates a new VirtualServer and registers cleanup with t.
func NewVirtualServer(t *testing.T) *VirtualServer {
	t.Helper()
	vs := &VirtualServer{
		scenarios: vmodel.NewGenericRegistry[Scenario](),
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
//
// **Provider Mock**: This is a provider mock, not a gateway. Routes are /v1/...
// (provider-native), not /tingly/... (gateway). The gateway transforms /tingly/{scenario}/v1/...
// requests to provider format before forwarding to this server.
func NewVirtualServerForCLI() *VirtualServer {
	vs := &VirtualServer{
		scenarios: vmodel.NewGenericRegistry[Scenario](),
	}

	mux := http.NewServeMux()

	// Provider-native routes (matching actual provider APIs)
	// Most providers require /v1/ prefix, but we register both for flexibility
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)

	// Google API route pattern: /v1beta/models/{model}/generateContent
	// Using catch-all handler since model name is dynamic in the path
	mux.HandleFunc("/v1beta/models/", vs.handleGoogle)

	vs.server = httptest.NewServer(mux)
	return vs
}

// RegisterScenario registers a scenario so the virtual server can serve its
// mock responses. If a scenario with the same name was previously registered
// it is replaced (the registry ordinarily errors on duplicate IDs, so we
// pre-clear).
func (vs *VirtualServer) RegisterScenario(s Scenario) {
	vs.scenarios.Unregister(s.Name)
	_ = vs.scenarios.Register(s)
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

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatOpenAIChat]
	if !ok {
		http.Error(w, "no openai_chat mock response for scenario "+scenario, http.StatusInternalServerError)
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
	scenario := vs.detectScenario(bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatOpenAIResponses]
	if !ok {
		// Fallback to FormatOpenAIChat for Responses API when not explicitly defined
		builder, ok = resp.MockResponses[FormatOpenAIChat]
		if !ok {
			http.Error(w, "no openai_responses or openai_chat mock response for scenario "+scenario, http.StatusInternalServerError)
			return
		}
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

func (vs *VirtualServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatAnthropic]
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

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatGoogle]
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
			remaining := part[len(prefix):] // "{target}-{scenario}"
			// Find the last "-" to separate target from scenario
			lastDash := strings.LastIndex(remaining, "-")
			var name string
			if lastDash > 0 {
				name = remaining[lastDash+1:]
			} else {
				name = remaining
			}
			// Strip trailing action suffix if any (e.g. ":generateContent" edge case)
			if idx := strings.IndexByte(name, ':'); idx >= 0 {
				name = name[:idx]
			}
			if vs.scenarios.Has(name) {
				return name
			}
		}
	}
	// Fallback to body parsing
	return vs.detectScenario(body)
}

// detectScenario extracts the scenario name from the request body's model field.
// The model field is expected to be "virtual-model-{target}-{scenario}" (set by SetupRoute).
// We extract just the scenario name for lookup.
// Falls back to the first registered scenario if extraction fails.
func (vs *VirtualServer) detectScenario(body []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err == nil {
		if model, ok := m["model"].(string); ok {
			const prefix = "virtual-model-"
			if strings.HasPrefix(model, prefix) {
				// Format: virtual-model-{target}-{scenario}
				// We need to extract the scenario name after the target
				remaining := model[len(prefix):] // "{target}-{scenario}"
				// Find the last "-" to separate target from scenario
				lastDash := strings.LastIndex(remaining, "-")
				if lastDash > 0 {
					name := remaining[lastDash+1:]
					if vs.scenarios.Has(name) {
						return name
					}
				}
			}
		}
	}
	// Fallback: return first registered scenario name
	for _, s := range vs.scenarios.List() {
		return s.Name
	}
	return ""
}

func (vs *VirtualServer) firstScenario() Scenario {
	for _, s := range vs.scenarios.List() {
		return s
	}
	return Scenario{}
}
