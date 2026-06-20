//go:build e2e
// +build e2e

package protocoltest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
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

// TestFailover_CrossStyle_AnthropicToOpenAI_RetriesAndSucceeds is the e2e proof
// of the lifted failover: the client sends an Anthropic request; the primary tier
// is an Anthropic-style provider that returns 500; the orchestrator re-transforms
// the request into OpenAI Chat wire format and fails over to an OpenAI-style
// fallback, which succeeds. The client gets a 200 assembled back into Anthropic
// shape. We additionally assert the fallback actually received an OpenAI-shaped
// body (captured by env.virtual) — proving the per-attempt re-transform, not just
// a model/provider swap.
func TestFailover_CrossStyle_AnthropicToOpenAI_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupCrossStyleFailoverRoute(
		t,
		protocol.TypeAnthropicV1,   // client speaks Anthropic
		protocol.APIStyleAnthropic, // primary tier is Anthropic-style (returns 500)
		protocol.TypeOpenAIChat,    // fallback tier is OpenAI Chat-style (succeeds)
		pt.TextScenario(),
		pt.FailMockPreContent500,
	)

	result := env.SendWithModel(t, protocol.TypeAnthropicV1, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus, "fallback (OpenAI) must serve a success after Anthropic primary 500")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "Anthropic primary hit exactly once")
	assert.NotEmpty(t, result.Content, "client must receive Anthropic-shaped content from the OpenAI fallback")

	// The fallback is OpenAI Chat: env.virtual must have received the request on
	// the /chat/completions endpoint, proving the request was re-transformed from
	// Anthropic into OpenAI wire format on the failover attempt.
	require.Greater(t, env.UpstreamEndpointHits(pt.EndpointChat), 0,
		"fallback must have been reached on the OpenAI Chat endpoint (re-transform happened)")
	up := env.UpstreamLastRequest(pt.EndpointChat)
	require.NotNil(t, up, "fallback request body must have been captured")
	body := up.JSON()
	assert.Equal(t, "virtual-model-text-chat", body["model"], "upstream model must be the fallback's backend model")
	assert.Contains(t, body, "messages", "upstream body must be OpenAI Chat shape (messages[]), not Anthropic")
}

// TestFailover_CrossStyle_OpenAIToAnthropic_RetriesAndSucceeds is the reverse
// direction: an OpenAI Chat client, an OpenAI-style primary that 500s, and an
// Anthropic-style fallback. Proves the request is re-transformed into Anthropic
// Messages wire format on the failover attempt.
func TestFailover_CrossStyle_OpenAIToAnthropic_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupCrossStyleFailoverRoute(
		t,
		protocol.TypeOpenAIChat,  // client speaks OpenAI Chat
		protocol.APIStyleOpenAI,  // primary tier is OpenAI-style (returns 500)
		protocol.TypeAnthropicV1, // fallback tier is Anthropic-style (succeeds)
		pt.TextScenario(),
		pt.FailMockPreContent500,
	)

	result := env.SendWithModel(t, protocol.TypeOpenAIChat, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus, "Anthropic fallback must serve a success after OpenAI primary 500")
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "OpenAI primary hit exactly once")
	assert.NotEmpty(t, result.Content, "client must receive OpenAI-shaped content from the Anthropic fallback")

	require.Greater(t, env.UpstreamEndpointHits(pt.EndpointAnthropic), 0,
		"fallback must have been reached on the Anthropic Messages endpoint (re-transform happened)")
	body := env.UpstreamLastRequest(pt.EndpointAnthropic).JSON()
	assert.Contains(t, body, "messages", "upstream body must be Anthropic Messages shape")
	assert.Contains(t, body, "max_tokens", "Anthropic wire format requires max_tokens")
}

// TestFailover_CrossStyle_Stream_AnthropicToOpenAI_RetriesAndSucceeds is the
// streaming counterpart: the primary 500s pre-content (gate stays buffered), the
// orchestrator re-transforms into OpenAI Chat and fails over, and the fallback's
// SSE stream commits the gate. The client receives an Anthropic SSE stream
// assembled from the OpenAI fallback.
func TestFailover_CrossStyle_Stream_AnthropicToOpenAI_RetriesAndSucceeds(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupCrossStyleFailoverRoute(
		t,
		protocol.TypeAnthropicV1,
		protocol.APIStyleAnthropic,
		protocol.TypeOpenAIChat,
		pt.StreamingTextScenario(),
		pt.FailMockPreContent500,
	)

	result := env.SendWithModel(t, protocol.TypeAnthropicV1, route.ModelName, true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, int64(1), route.PrimaryCallCount.Load(), "Anthropic primary hit exactly once")
	assert.NotEmpty(t, result.StreamEvents, "client must receive an Anthropic SSE stream from the OpenAI fallback")
	require.Greater(t, env.UpstreamEndpointHits(pt.EndpointChat), 0,
		"streaming fallback must have been reached on the OpenAI Chat endpoint")
}

// TestFailover_CrossStyle_SetupError_AdvancesToFallback proves the
// "retryable, advance" decision for in-attempt setup failures. The primary is a
// Google-style provider on the Responses path; ResponsesCreate rejects Google
// during target resolution, which now routes through failAttemptSetup as a
// retryable 500 instead of terminating the request. The loop must advance to the
// OpenAI fallback. The Google primary is never actually called — the error
// precedes any upstream request.
func TestFailover_CrossStyle_SetupError_AdvancesToFallback(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupCrossStyleFailoverRoute(
		t,
		protocol.TypeOpenAIResponses,
		protocol.APIStyleGoogle, // primary: unsupported for Responses → in-attempt setup error
		protocol.TypeOpenAIChat, // fallback: OpenAI Chat (succeeds)
		pt.TextScenario(),
		"setup-error-primary",
	)

	result := env.SendWithModel(t, protocol.TypeOpenAIResponses, route.ModelName, false)

	require.Equal(t, 200, result.HTTPStatus, "loop must advance past the setup-error primary to the fallback")
	assert.Equal(t, int64(0), route.PrimaryCallCount.Load(),
		"Google primary must NOT be called — the setup error precedes any upstream request")
	assert.NotEmpty(t, result.Content)
	require.Greater(t, env.UpstreamEndpointHits(pt.EndpointChat), 0, "fallback OpenAI Chat must have served the request")
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
