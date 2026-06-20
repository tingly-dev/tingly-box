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

// TestPassthroughStream_FreesBodyWhileStreamingE2E proves the parsed request is
// GC'd mid-stream on the real Anthropic-beta passthrough, at the failover gate's
// first-chunk commit (releaseReqCtxAfterStreamCommit).
//
// Two services are required: only then does the failover closure keep reqCtx
// reachable for the whole stream, so the release actually matters (single-service
// lets GC drop it on its own). The upstream delivers opening chunks to commit the
// gate, then parks the stream so we measure after the release has fired.
//
// Measured (64 MB body): ~128 MB reclaimed mid-stream with the release, ~0 without.
// One body (~64 MB) always remains — the SDK's marshaled copy held by
// http.Request.GetBody for the connection lifetime, which no reference drop reaches.
func TestPassthroughStream_FreesBodyWhileStreamingE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const fillerMB = 64
	const fillerBytes = fillerMB << 20

	started := make(chan struct{})
	release := make(chan struct{})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain and drop the inbound request body so the upstream side does not
		// pin it — we are measuring the proxy's retention, not the test server's.
		_, _ = io.Copy(io.Discard, r.Body)

		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl.Flush() // headers out -> ForwardAnthropicV1BetaStream returns

		// Deliver the opening events so the proxy writes its first client chunk
		// and the failover gate commits — the point at which reqCtx is released.
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_e2e","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":3,"output_tokens":0}}}`)
		fl.Flush()
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		fl.Flush()
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
		fl.Flush()

		close(started)
		<-release // gate has committed; park the stream open before finishing

		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer backend.Close()

	s := newMCPDisabledTestServer(t)
	provider := &typ.Provider{
		UUID:     "p-passthrough-release",
		Name:     "p-passthrough-release",
		APIStyle: protocol.APIStyleAnthropic,
		APIBase:  backend.URL,
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

	// Parse a large body through the SDK so the gjson decoder pins the raw JSON
	// onto the parsed struct's metadata — reproducing the profiled retention.
	body := buildBigBetaBody(t, fillerBytes)
	var parsed protocol.AnthropicBetaMessagesRequest
	require.NoError(t, json.Unmarshal(body, &parsed))
	body = nil //nolint:ineffassign // drop our handle on the raw bytes; only the parsed struct pins it now

	// Baseline live heap while the parsed request (and the body it pins) is held.
	var m0 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	t.Logf("baseline (parsed body held) = %.1f MB", float64(m0.HeapAlloc)/(1024*1024))

	// Two services force the failover branch, which keeps reqCtx reachable for the
	// whole stream — the case the release targets (single-service GC's it anyway).
	rule := &typ.Rule{
		Scenario: typ.ScenarioOpenAI,
		Services: []*loadbalance.Service{
			{Provider: provider.UUID, Model: "worker-model", Active: true, Weight: 1},
			{Provider: provider.UUID, Model: "worker-model-2", Active: true, Weight: 1},
		},
	}
	w := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", strings.NewReader("{}"))

	done := make(chan struct{})
	go func(req protocol.AnthropicBetaMessagesRequest) {
		defer close(done)
		s.AnthropicMessagesV1Beta(c, req, "proxy-model", provider, "worker-model", rule)
	}(parsed)
	// The test no longer holds the parsed request: only the goroutine's value
	// copy (and whatever the proxy retains) keeps it alive from here on.
	parsed = protocol.AnthropicBetaMessagesRequest{}

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

	// The stream is parked open past the gate commit. Poll: the commit-time
	// release plus Go liveness on the dead upstream copies should let GC reclaim
	// the ~fillerMB parsed request now, mid-stream.
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
			t.Fatalf("body NOT freed mid-stream: reclaimed only %.1f MB, expected ~%d MB — a reference still pins the parsed request after the gate committed", freed, fillerMB)
		case <-time.After(25 * time.Millisecond):
		}
	}

	t.Logf("reclaimed during stream = %.1f MB", freed)

	close(release)
	<-done

	t.Logf("final floor (request done) = %.1f MB", heapMB())

	require.Greater(t, freed, float64(fillerMB)*0.6,
		"the parsed request body must be reclaimable during the stream")
	require.Equal(t, http.StatusOK, w.Code)
}

// buildBigBetaBody produces a valid Anthropic beta messages request whose user
// message carries `fillerBytes` of text, so the parsed request retains a large,
// easily-measurable chunk of heap.
func buildBigBetaBody(t *testing.T, fillerBytes int) []byte {
	t.Helper()
	m := map[string]any{
		"model":      "worker-model",
		"max_tokens": 256,
		"stream":     true,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": strings.Repeat("x", fillerBytes)},
				},
			},
		},
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}
