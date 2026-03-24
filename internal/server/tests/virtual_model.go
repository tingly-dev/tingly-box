package tests

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// VirtualModelConfig configures delay ranges for the virtual model.
type VirtualModelConfig struct {
	// Delay before the first SSE chunk is sent (simulates TTFT).
	MinFirstTokenDelayMs int
	MaxFirstTokenDelayMs int

	// Delay between the last content chunk and [DONE] (controls stream duration / TPS).
	MinEndDelayMs int
	MaxEndDelayMs int
}

// DefaultVirtualModelConfig returns sensible defaults (50–500ms TTFT, 100–1000ms end delay).
func DefaultVirtualModelConfig() VirtualModelConfig {
	return VirtualModelConfig{
		MinFirstTokenDelayMs: 50,
		MaxFirstTokenDelayMs: 500,
		MinEndDelayMs:        100,
		MaxEndDelayMs:        1000,
	}
}

// virtualModelChunks is the fixed streaming content split into chunks.
var virtualModelChunks = []string{
	"I am ",
	"testing ",
	"and random sleep",
}

const (
	virtualModelID      = "chatcmpl-virtual"
	virtualModelName    = "virtual-model"
	virtualInputTokens  = 10
	virtualOutputTokens = 7
)

// VirtualModel is a local OpenAI-compatible HTTP server with configurable random delays.
// It accepts any chat completions request, streams a fixed response, and produces
// realistic TTFT / TPS / latency values for metrics E2E testing.
type VirtualModel struct {
	server *httptest.Server
	cfg    VirtualModelConfig
	rng    *rand.Rand
}

// NewVirtualModel creates and starts a VirtualModel with default config.
func NewVirtualModel() *VirtualModel {
	return NewVirtualModelWithConfig(DefaultVirtualModelConfig())
}

// NewVirtualModelWithConfig creates and starts a VirtualModel with a custom config.
func NewVirtualModelWithConfig(cfg VirtualModelConfig) *VirtualModel {
	vm := &VirtualModel{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", vm.handleChatCompletions)
	mux.HandleFunc("/chat/completions", vm.handleChatCompletions)
	vm.server = httptest.NewServer(mux)

	return vm
}

// URL returns the base URL of the embedded server.
func (vm *VirtualModel) URL() string {
	return vm.server.URL
}

// Provider returns a *typ.Provider configured to point to this virtual model.
// name is used as both UUID and Name; must be unique within a test's config.
func (vm *VirtualModel) Provider(name string) *typ.Provider {
	return &typ.Provider{
		UUID:     name,
		Name:     name,
		APIBase:  vm.server.URL,
		APIStyle: protocol.APIStyleOpenAI,
		Token:    "virtual-model-token",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
	}
}

// Close shuts down the embedded server.
func (vm *VirtualModel) Close() {
	vm.server.Close()
}

// randDelay sleeps for a random duration in [minMs, maxMs].
func (vm *VirtualModel) randDelay(minMs, maxMs int) {
	if minMs >= maxMs {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	ms := minMs + vm.rng.Intn(maxMs-minMs+1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func (vm *VirtualModel) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	stream, _ := req["stream"].(bool)
	if stream {
		vm.handleStreaming(w)
	} else {
		vm.handleNonStreaming(w)
	}
}

func (vm *VirtualModel) handleStreaming(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	now := time.Now().Unix()

	// First-token delay — applied before any chunk so the proxy's OnStreamEvent
	// hook records SetFirstTokenTime after the full delay, giving a meaningful TTFT.
	vm.randDelay(vm.cfg.MinFirstTokenDelayMs, vm.cfg.MaxFirstTokenDelayMs)

	// Role chunk — first event, triggers SetFirstTokenTime in the proxy.
	roleChunk := map[string]interface{}{
		"id":      virtualModelID,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   virtualModelName,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{"role": "assistant", "content": ""},
				"finish_reason": nil,
			},
		},
	}
	vm.writeSSEChunk(w, flusher, roleChunk)

	// Content chunks.
	for _, text := range virtualModelChunks {
		chunk := map[string]interface{}{
			"id":      virtualModelID,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   virtualModelName,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{"content": text},
					"finish_reason": nil,
				},
			},
		}
		vm.writeSSEChunk(w, flusher, chunk)
	}

	// Final chunk with usage (finish_reason=stop).
	finalChunk := map[string]interface{}{
		"id":      virtualModelID,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   virtualModelName,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     virtualInputTokens,
			"completion_tokens": virtualOutputTokens,
			"total_tokens":      virtualInputTokens + virtualOutputTokens,
		},
	}
	vm.writeSSEChunk(w, flusher, finalChunk)

	// End delay — determines TPS (7 output tokens / stream duration).
	vm.randDelay(vm.cfg.MinEndDelayMs, vm.cfg.MaxEndDelayMs)

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (vm *VirtualModel) handleNonStreaming(w http.ResponseWriter) {
	// Single delay before returning (maps to total latency; TTFT falls back to latency).
	vm.randDelay(vm.cfg.MinFirstTokenDelayMs, vm.cfg.MaxFirstTokenDelayMs)

	resp := map[string]interface{}{
		"id":      virtualModelID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   virtualModelName,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "I am testing and random sleep",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     virtualInputTokens,
			"completion_tokens": virtualOutputTokens,
			"total_tokens":      virtualInputTokens + virtualOutputTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (vm *VirtualModel) writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}
