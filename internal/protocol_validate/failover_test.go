//go:build e2e
// +build e2e

package protocol_validate_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	pt "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestFailover_Nonstream_429_RetriesAndSucceeds verifies the priority routing
// tactic: primary tier returns 429, dispatch discards the buffered error and
// retries the fallback tier, client receives a 200 from the fallback.
func TestFailover_Nonstream_429_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.TextScenario(), http.StatusTooManyRequests)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus, "fallback must serve a success after primary 429")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary tier should be hit exactly once")
	assert.NotEmpty(t, result.Content, "fallback must produce real content")
}

// TestFailover_Nonstream_500_RetriesAndSucceeds is the symmetric 5xx case;
// retryableUpstreamStatuses includes 500 so this exercises the same path.
func TestFailover_Nonstream_500_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.TextScenario(), http.StatusInternalServerError)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load())
	assert.NotEmpty(t, result.Content)
}

// TestFailover_Stream_PreContent_429_RetriesAndSucceeds is the streaming
// counterpart. The primary returns plain JSON with 429 (no SSE), so the gate
// stays buffered (CommitFirstChunk never fires); the orchestrator sees
// gate.Status()=429, discards, retries fallback. Fallback's streaming response
// commits the gate on the first real SSE event and the client receives the
// stream.
func TestFailover_Stream_PreContent_429_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), http.StatusTooManyRequests)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents, "fallback must produce an SSE stream")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load())
}

// TestFailover_Stream_PreContent_500_RetriesAndSucceeds — same shape but
// against a 500. 500 is in retryableUpstreamStatuses specifically because
// SendStreamingError emits 500 for pre-stream errors.
func TestFailover_Stream_PreContent_500_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), http.StatusInternalServerError)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load())
}

// TestFailover_AllTiersFail_ClientSeesLastError — both services return 429.
// After the loop exhausts the candidate pool, the deferred CommitIfBuffered
// flushes the last buffered error to the wire. The client must see a non-200,
// not a hung connection or a panic.
func TestFailover_AllTiersFail_ClientSeesLastError(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	failHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"all tiers down","type":"upstream_error","code":"failover_test"}}`)
	})

	route := env.SetupCustomFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, failHandler, failHandler, "all-fail")

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	assert.NotEqual(t, 200, result.HTTPStatus, "no tier produced a success — client must not see a 200")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary attempted once")
	assert.Equal(t, int64(1), route.FallbackCallCount.Load(), "fallback attempted once")
}

// TestFailover_MidStream_NoRetry_GateCommitted is the critical safety guarantee:
// once CommitFirstChunk has flushed the first real chunk to the wire, retry is
// impossible. The primary sends one valid SSE delta, flushes, then hijacks and
// closes the TCP connection. The gateway has already passed the first chunk
// through — gate.Committed() is true. The orchestrator must observe this and
// NOT retry the fallback.
func TestFailover_MidStream_NoRetry_GateCommitted(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	midstreamFailHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = fmt.Fprint(w,
			"data: {\"id\":\"chatcmpl-x\",\"object\":\"chat.completion.chunk\","+
				"\"created\":1,\"model\":\"gpt-4o\","+
				"\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	})

	// Fallback handler — should NOT be invoked because the gate committed
	// on the primary's first chunk.
	fallbackHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"id\":\"FALLBACK\",\"choices\":[]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	})

	route := env.SetupCustomFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, midstreamFailHandler, fallbackHandler, "mid-stream")

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus, "first chunk committed → client sees 200")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary attempted once")
	assert.Equal(t, int64(0), route.FallbackCallCount.Load(), "fallback MUST NOT be retried after gate commit")
	assert.NotEmpty(t, result.StreamEvents, "client must have received the committed first chunk")
}

// TestFailover_SingleService_Bypass — single-service rules bypass the gate
// entirely (orchestrator's len(activeServices) <= 1 short-circuit). The
// existing SetupRoute path exercises this — assertion is just that the
// streaming path still produces a clean SSE 200, proving the bypass didn't
// regress alongside the gate refactor.
func TestFailover_SingleService_Bypass(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}
