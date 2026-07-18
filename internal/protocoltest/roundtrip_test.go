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

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "end_turn", result.FinishReason)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

func TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_OpenAIChat_To_AnthropicV1_Text(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, pt.TextScenario())

	result := env.SendAs(t, protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, pt.TextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "stop", result.FinishReason)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
	assert.NotEmpty(t, result.ToolCalls[0].ID)
	assert.Contains(t, result.ToolCalls[0].Arguments, "location")
}

func TestRoundTrip_AnthropicBeta_To_OpenAIChat_ToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.ToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.ToolUseScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

func TestRoundTrip_AnthropicV1_To_AnthropicV1_Thinking(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pt.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pt.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ThinkingContent)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Thinking(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_MultiTurn(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.MultiTurnScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.MultiTurnScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicV1_To_OpenAIChat_Streaming(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.StreamingTextScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.NotEmpty(t, result.Content)
}

func TestRoundTrip_AnthropicBeta_To_OpenAIChat_StreamingToolUse(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.StreamingToolUseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, pt.StreamingToolUseScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "get_weather", result.ToolCalls[0].Name)
}

func TestRoundTrip_ErrorPassthrough(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, pt.ErrorScenario(), false)

	assert.NotEqual(t, 200, result.HTTPStatus)
}

// AnthropicBeta → OpenAIResponses is the path where the streaming
// first-event prime lives (see internal/protocol/stream/prime.go).
// The happy-path test exercises prime + replay wrapper end-to-end:
// the gateway forces the upstream's first SSE event, hands a wrapped
// iterator off to the handler, and the handler converts the rest of
// the Responses-API events into Anthropic Messages SSE frames.
func TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Streaming(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.StreamingTextScenario(), true)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.StreamEvents)
	assert.Contains(t, result.Content, "Paris")
}

// Pre-stream prime failure: ErrorScenario's streaming branch returns
// `data: {"error":...}` as the first SSE line. The SDK's Stream errors
// out on its first Next() call (gjson "error" key detection). Priming
// surfaces that as a non-2xx — the buffered failover writer
// captures it, and since there's only one service in the rule the
// captured error commits as the terminal reply. The client sees a
// real 500 with a JSON error body, not a 200 with a malformed SSE
// stream that includes an upstream error event.
func TestRoundTrip_StreamingPrimeFailure_To_OpenAIResponses(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.ErrorScenario(), true)

	// The HTTP status must reflect the upstream failure rather than
	// silently 200-with-error-event. 500 is what SendStreamingError
	// emits, which isRetryableStatus accepts; if either side flips to
	// 200 the buffered writer's promotion logic broke.
	assert.Equal(t, 500, result.HTTPStatus,
		"pre-stream prime failure must surface as a 5xx, not a 200 SSE")
	// Parsed assistant content should be empty — no real upstream
	// content ever streamed, so the handler had nothing to convert.
	assert.Empty(t, result.Content,
		"no assistant content should be assembled from a prime-failed stream")
}

// Anthropic-native passthrough: client and provider both speak Anthropic,
// so the request flows through HandleAnthropicBeta (ProcessStream over the
// Anthropic SDK stream). A pre-content upstream error must surface as a
// retryable 5xx, not a 200 SSE error event — the in-line !Written guard in
// the passthrough converter + ProcessStream's no-empty-flush. This is the
// common multi-Anthropic-account failover shape.
func TestRoundTrip_StreamingPreContentFailure_AnthropicNative(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, pt.ErrorScenario(), true)

	assert.Equal(t, 500, result.HTTPStatus,
		"Anthropic-native pre-content failure must surface as a 5xx, not a 200 SSE")
	assert.Empty(t, result.Content,
		"no assistant content should be assembled from a failed pre-content stream")
}

// ---- Codex assembly: nonstream client / stream upstream / assemble ----
//
// Codex only speaks the streaming Responses API. A non-streaming Anthropic
// client request against a Codex-flagged provider is routed by
// dispatchOpenAIResponses (provider.IsCodexProvider()) through
// forwardResponsesStream + PrimeResponsesStream + the assembly handler
// instead of a plain non-streaming forward — the third cell of the
// {v1,beta} × {nonstream,stream,assemble} Responses→Anthropic matrix (see
// internal/server/protocol_cross.go). Before SetupCodexAssemblyRoute, this
// cell was unreachable by the harness: the routing check and the client's
// dial target were the same provider.APIBase field, so a route could never
// point at a local VirtualServer while also tripping the Codex branch.

// TestRoundTrip_CodexAssembly_Golden proves the happy path of the
// previously-unreachable cell: a non-streaming Anthropic v1 request against
// a Codex-flagged provider gets a well-formed 200 message, assembled from a
// real (mocked) upstream SSE stream via the genuine dispatch decision.
func TestRoundTrip_CodexAssembly_Golden(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicV1, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, pt.StreamingTextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Equal(t, "assistant", result.Role)
	assert.Contains(t, result.Content, "Paris")
}

// TestRoundTrip_CodexAssembly_Beta mirrors the golden case for the Anthropic
// beta source, exercising assembleResponsesToAnthropicBeta instead of the v1
// variant.
func TestRoundTrip_CodexAssembly_Beta(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicBeta, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.StreamingTextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Contains(t, result.Content, "Paris")
}

// TestRoundTrip_CodexAssembly_PrimeFailure covers the same pre-stream
// failure this file's TestRoundTrip_StreamingPrimeFailure_To_OpenAIResponses
// covers for the plain streaming branch, but for the assembly branch: a
// client that asked for a non-streaming response must still get a JSON
// error with the upstream's status, not a 200 or an SSE frame, when the
// mocked upstream fails before any event is readable.
func TestRoundTrip_CodexAssembly_PrimeFailure(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicV1, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, pt.ErrorScenario(), false)

	assert.GreaterOrEqual(t, result.HTTPStatus, 400,
		"a mocked upstream failure must surface as a 4xx/5xx, not a 200")
	assert.Empty(t, result.Content,
		"no assistant content should be assembled from a prime-failed stream")
}

// TestRoundTrip_CodexAssembly_NoContentBlocks reproduces #1316's actual
// repro end to end: the upstream starts a normal 200 stream (some events
// arrive) but is cut before any content block completes — no
// response.output_text.done, no response.completed. ErrorMidStreamCloseScenario
// already models exactly this shape for FormatOpenAIResponses (see
// buildMidStreamTruncated in vmodel/benchmark/scenario/scenario.go), so this
// reuses it rather than hand-building synthetic events. Before #1316's fix,
// this folded into a 200 message with content:null; the fix requires a
// retryable error instead.
func TestRoundTrip_CodexAssembly_NoContentBlocks(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicV1, pt.ErrorMidStreamCloseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, pt.ErrorMidStreamCloseScenario(), false)

	assert.GreaterOrEqual(t, result.HTTPStatus, 400,
		"a stream cut before any content block completes must fail, not return 200 with content:null")
}

func TestRoundTrip_AllSources_TextScenario_NonStreaming(t *testing.T) {
	sources := []protocol.APIType{
		protocol.TypeAnthropicV1,
		protocol.TypeAnthropicBeta,
		protocol.TypeOpenAIChat,
		protocol.TypeOpenAIResponses,
	}

	for _, src := range sources {
		src := src
		t.Run(string(src), func(t *testing.T) {
			t.Parallel()
			env := pt.NewTestEnv(t)
			defer env.Close()

			env.SetupRoute(src, protocol.TypeOpenAIChat, pt.TextScenario())

			result := env.SendAs(t, src, protocol.TypeOpenAIChat, pt.TextScenario(), false)
			require.Equal(t, 200, result.HTTPStatus)
			assert.NotEmpty(t, result.Content)
		})
	}
}
