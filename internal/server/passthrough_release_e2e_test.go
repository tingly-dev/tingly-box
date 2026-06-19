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

// TestPassthroughStream_FreesBodyWhileStreamingE2E is the end-to-end proof that
// ReleaseRequest lets the parsed request be GC'd *while the stream is still open*
// on the real Anthropic-beta passthrough path — not just at the isolated dispatch
// level covered by TestPassthroughRelease_FreesRequestAfterForward.
//
// WHY THE FAILOVER PATH MATTERS (and is what we test): the parsed request reaches
// the stream loop via reqCtx, which is captured by the failover attempt closure
// passed to dispatchWithPriorityFailover. With a *single* active service that
// branch calls attempt() once and returns, so the closure (and reqCtx) is dead
// while the stream runs and Go's GC drops the parsed request on its own — the fix
// is redundant there. With *two or more* active services the failover loop keeps
// the attempt closure live across iterations, so reqCtx stays reachable for the
// whole stream; without ReleaseRequest the parsed request (and the entire body
// the SDK gjson decoder pins onto it) is retained until the stream ends. This
// test uses two services so it exercises exactly that case.
//
// Measured contrast (64 MB body, this test):
//   - without ReleaseRequest: ~0 MB reclaimed during the stream (parsed request,
//     ~128 MB after gjson amplification, stays live the whole time)
//   - with ReleaseRequest:    ~128 MB reclaimed during the stream
//
// One body's worth (~64 MB) always remains live during the stream regardless:
// that is the SDK's own marshaled copy, captured by http.Request.GetBody for the
// connection lifetime (see requestconfig.Execute) — a separate retention that no
// reference drop on our side can reach.
//
// Mechanism:
//  1. Build a large request, parse it through the SDK (gjson pins the body).
//  2. Drive the real AnthropicMessagesV1Beta in a goroutine with a 2-service rule.
//  3. The fake upstream flushes response headers (so ForwardAnthropicV1BetaStream
//     returns and the dispatch runs ReleaseRequest), then BLOCKS before sending
//     any SSE data — parking the stream handler with reqCtx still reachable via
//     the live failover closure.
//  4. While the stream is blocked open, force GC and confirm the parsed request
//     has been reclaimed from the live heap.
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

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // headers out -> ForwardAnthropicV1BetaStream returns

		close(started)
		<-release // hold the stream open so the proxy is parked in its read loop

		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_e2e","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":3,"output_tokens":0}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
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

	// Two active services so dispatchWithPriorityFailover takes its failover
	// branch, which keeps the attempt closure (capturing reqCtx) live across the
	// whole loop — and thus reqCtx reachable for the entire stream. This is the
	// case where reqCtx pins the parsed request during streaming, so it is the
	// case the ReleaseRequest fix is meant to address. (With a single service the
	// closure dies immediately and Go's GC drops reqCtx on its own.)
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

	// The stream is parked open. Poll: if the fix works the dead upstream copies
	// and the nilled dispatch references let GC reclaim the ~fillerMB body now.
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
			t.Fatalf("body NOT freed while stream open: reclaimed only %.1f MB, expected ~%d MB — an upstream reference still pins the parsed request during the stream", freed, fillerMB)
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
