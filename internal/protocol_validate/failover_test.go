//go:build e2e
// +build e2e

package protocol_validate_test

import (
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

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.TextScenario(), pt.FailMockPreContent429)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus, "fallback must serve a success after primary 429")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary tier should be hit exactly once")
	assert.NotEmpty(t, result.Content, "fallback must produce real content")
}

// TestFailover_Nonstream_500_RetriesAndSucceeds is the symmetric 5xx case.
func TestFailover_Nonstream_500_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.TextScenario(), pt.FailMockPreContent500)

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

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), pt.FailMockPreContent429)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents, "fallback must produce an SSE stream")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load())
}

// TestFailover_Stream_PreContent_500_RetriesAndSucceeds — same shape but
// against a 500.
func TestFailover_Stream_PreContent_500_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), pt.FailMockPreContent500)

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

	route := env.SetupBothFailingRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.FailMockPreContent429)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	assert.NotEqual(t, 200, result.HTTPStatus, "no tier produced a success — client must not see a 200")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary attempted once")
	assert.Equal(t, int64(1), route.FallbackCallCount.Load(), "fallback attempted once")
}

// TestFailover_MidStream_NoRetry_GateCommitted is the critical safety guarantee:
// once CommitFirstChunk has flushed the first real chunk to the wire, retry is
// impossible. The primary emits a real chunk then closes the connection mid
// stream (vmodel's virtual-fail-midstream-close mock). The gateway has already
// passed the first chunk through — gate.Committed() is true. The orchestrator
// must observe this and NOT retry the fallback. We verify by content
// discrimination: the primary mock's content is fixed and distinct from the
// fallback scenario's content.
func TestFailover_MidStream_NoRetry_GateCommitted(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupFailoverRoute(t, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), pt.FailMockMidStreamCut)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus, "first chunk committed → client sees 200")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "primary attempted once")
	require.NotEmpty(t, result.StreamEvents, "client must have received the committed first chunk")
	// Primary mock's content starts with "hello world" (see vmodel.ErrorMockSpecs).
	// Fallback's StreamingTextScenario emits a different known string. If the
	// client received primary's content, fallback was not retried — gate stayed
	// committed.
	assert.Contains(t, result.Content, "hello world", "client must see primary's truncated content, not fallback's")
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
