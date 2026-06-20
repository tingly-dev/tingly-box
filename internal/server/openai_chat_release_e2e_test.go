package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestOpenAIChatPassthroughStream_FreesBodyMidStreamE2E is the OpenAI-chat
// counterpart of the beta test. It proves the OpenAI-specific change works:
// HandleOpenAIChatStream takes a pre-computed estimate instead of the request, so
// (with the commit-time reqCtx release) the body is reclaimed mid-stream. Without
// it, streamOpenAIChat would keep req for the end-of-stream estimate fallback.
// Two services force the failover branch; measured ~64 MB reclaimed mid-stream.
func TestOpenAIChatPassthroughStream_FreesBodyMidStreamE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const fillerMB = 64
	const fillerBytes = fillerMB << 20

	started := make(chan struct{})
	release := make(chan struct{})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)

		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl.Flush()

		// Opening chunks so the proxy writes its first client chunk and the
		// failover gate commits (the point at which reqCtx is released).
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n"))
		fl.Flush()

		close(started)
		<-release // gate has committed; park the stream open

		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer backend.Close()

	s := newMCPDisabledTestServer(t)
	provider := &typ.Provider{
		UUID:     "p-openai-passthrough-release",
		Name:     "p-openai-passthrough-release",
		APIStyle: protocol.APIStyleOpenAI,
		APIBase:  backend.URL + "/v1",
		Token:    "k",
		Enabled:  true,
	}

	heapMB := func() float64 {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return float64(m.HeapAlloc) / (1024 * 1024)
	}
	t.Logf("floor (server only, no body) = %.1f MB", heapMB())

	// Parse a large body through the SDK so the gjson decoder pins the raw JSON.
	body := buildBigOpenAIChatBody(t, fillerBytes)
	var parsed protocol.OpenAIChatCompletionRequest
	require.NoError(t, json.Unmarshal(body, &parsed))
	body = nil //nolint:ineffassign // only the parsed struct pins the body now

	var m0 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	t.Logf("baseline (parsed body held) = %.1f MB", float64(m0.HeapAlloc)/(1024*1024))

	rule := &typ.Rule{
		Scenario: typ.ScenarioOpenAI,
		Services: []*loadbalance.Service{
			{Provider: provider.UUID, Model: "worker-model", Active: true, Weight: 1},
			{Provider: provider.UUID, Model: "worker-model-2", Active: true, Weight: 1},
		},
	}
	w := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))

	done := make(chan struct{})
	go func(req protocol.OpenAIChatCompletionRequest) {
		defer close(done)
		s.OpenAIChatCompletion(c, req, "worker-model", provider, typ.ScenarioOpenAI, rule)
	}(parsed)
	parsed = protocol.OpenAIChatCompletionRequest{}

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("upstream never received the forwarded request")
	}

	freedMB := func() float64 {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return (float64(m0.HeapAlloc) - float64(m.HeapAlloc)) / (1024 * 1024)
	}

	var freed float64
	deadline := time.After(5 * time.Second)
poll:
	for {
		freed = freedMB()
		if freed > float64(fillerMB)*0.6 {
			break poll
		}
		select {
		case <-deadline:
			close(release)
			<-done
			t.Fatalf("body NOT freed mid-stream: reclaimed only %.1f MB, expected ~%d MB — the OpenAI chat handler still pins the parsed request after the gate committed", freed, fillerMB)
		case <-time.After(25 * time.Millisecond):
		}
	}

	t.Logf("reclaimed during stream = %.1f MB", freed)

	close(release)
	<-done

	t.Logf("final floor (request done) = %.1f MB", heapMB())

	require.Greater(t, freed, float64(fillerMB)*0.6,
		"the parsed OpenAI chat request body must be reclaimable mid-stream")
}

// buildBigOpenAIChatBody produces a valid OpenAI chat completion request whose
// user message carries fillerBytes of text.
func buildBigOpenAIChatBody(t *testing.T, fillerBytes int) []byte {
	t.Helper()
	m := map[string]any{
		"model":  "worker-model",
		"stream": true,
		"messages": []any{
			map[string]any{"role": "user", "content": strings.Repeat("x", fillerBytes)},
		},
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}
