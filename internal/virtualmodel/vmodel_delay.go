package virtualmodel

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

// DelayConfig configures the random delay ranges for a DelayProvider.
type DelayConfig struct {
	// Delay before the first SSE chunk is emitted (controls TTFT).
	MinFirstTokenDelayMs int
	MaxFirstTokenDelayMs int

	// Delay between the last content chunk and [DONE] (controls stream duration / TPS).
	MinEndDelayMs int
	MaxEndDelayMs int
}

// DefaultDelayConfig returns sensible defaults: 50–500ms TTFT, 100–1000ms end delay.
func DefaultDelayConfig() DelayConfig {
	return DelayConfig{
		MinFirstTokenDelayMs: 50,
		MaxFirstTokenDelayMs: 500,
		MinEndDelayMs:        100,
		MaxEndDelayMs:        1000,
	}
}

// delayModelChunks is the fixed response content split into SSE chunks.
var delayModelChunks = []string{
	"I am ",
	"testing ",
	"and random sleep",
}

const (
	delayModelID         = "chatcmpl-delay"
	delayModelResponseID = "delay-model"
	delayInputTokens     = 10
	delayOutputTokens    = 7
)

// DelayProvider is an embedded OpenAI-compatible HTTP server with configurable
// random delays. It accepts any chat completions request, streams the fixed
// response "I am testing and random sleep", and produces realistic TTFT / TPS /
// latency values for metrics E2E testing.
//
// Use it as a typ.Provider in routing rules so requests flow through the full
// proxy pipeline (routing → OpenAI client → metrics tracking).
type DelayProvider struct {
	server *httptest.Server
	cfg    DelayConfig
	rng    *rand.Rand
}

// NewDelayProvider creates and starts a DelayProvider with default config.
func NewDelayProvider() *DelayProvider {
	return NewDelayProviderWithConfig(DefaultDelayConfig())
}

// NewDelayProviderWithConfig creates and starts a DelayProvider with a custom config.
func NewDelayProviderWithConfig(cfg DelayConfig) *DelayProvider {
	dp := &DelayProvider{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	mux := http.NewServeMux()
	// Handle both with and without /v1 prefix since the OpenAI SDK appends /chat/completions.
	mux.HandleFunc("/v1/chat/completions", dp.handle)
	mux.HandleFunc("/chat/completions", dp.handle)
	dp.server = httptest.NewServer(mux)
	return dp
}

// URL returns the base URL of the embedded server.
func (dp *DelayProvider) URL() string {
	return dp.server.URL
}

// Provider returns a *typ.Provider configured to route to this delay server.
// name is used as both UUID and Name; it must be unique within the test config.
func (dp *DelayProvider) Provider(name string) *typ.Provider {
	return &typ.Provider{
		UUID:     name,
		Name:     name,
		APIBase:  dp.server.URL,
		APIStyle: protocol.APIStyleOpenAI,
		Token:    "delay-provider-token",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
	}
}

// Close shuts down the embedded HTTP server.
func (dp *DelayProvider) Close() {
	dp.server.Close()
}

func (dp *DelayProvider) randDelay(minMs, maxMs int) {
	if minMs <= 0 && maxMs <= 0 {
		return
	}
	if minMs >= maxMs {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	ms := minMs + dp.rng.Intn(maxMs-minMs+1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func (dp *DelayProvider) handle(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	stream, _ := req["stream"].(bool)
	if stream {
		dp.handleStreaming(w)
	} else {
		dp.handleNonStreaming(w)
	}
}

func (dp *DelayProvider) handleStreaming(w http.ResponseWriter) {
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
	dp.randDelay(dp.cfg.MinFirstTokenDelayMs, dp.cfg.MaxFirstTokenDelayMs)

	// Role chunk — first event, triggers SetFirstTokenTime in the proxy.
	dp.writeSSE(w, flusher, map[string]interface{}{
		"id": delayModelID, "object": "chat.completion.chunk", "created": now, "model": delayModelResponseID,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": ""}, "finish_reason": nil},
		},
	})

	// Content chunks.
	for _, text := range delayModelChunks {
		dp.writeSSE(w, flusher, map[string]interface{}{
			"id": delayModelID, "object": "chat.completion.chunk", "created": now, "model": delayModelResponseID,
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]interface{}{"content": text}, "finish_reason": nil},
			},
		})
	}

	// Final chunk with usage (finish_reason=stop).
	dp.writeSSE(w, flusher, map[string]interface{}{
		"id": delayModelID, "object": "chat.completion.chunk", "created": now, "model": delayModelResponseID,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "stop"},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     delayInputTokens,
			"completion_tokens": delayOutputTokens,
			"total_tokens":      delayInputTokens + delayOutputTokens,
		},
	})

	// End delay — determines effective TPS (delayOutputTokens / stream duration).
	dp.randDelay(dp.cfg.MinEndDelayMs, dp.cfg.MaxEndDelayMs)

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (dp *DelayProvider) handleNonStreaming(w http.ResponseWriter) {
	// Single delay before returning (TTFT falls back to total latency).
	dp.randDelay(dp.cfg.MinFirstTokenDelayMs, dp.cfg.MaxFirstTokenDelayMs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": delayModelID, "object": "chat.completion", "created": time.Now().Unix(), "model": delayModelResponseID,
		"choices": []map[string]interface{}{
			{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "I am testing and random sleep"}, "finish_reason": "stop"},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     delayInputTokens,
			"completion_tokens": delayOutputTokens,
			"total_tokens":      delayInputTokens + delayOutputTokens,
		},
	})
}

func (dp *DelayProvider) writeSSE(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}
