package protocoltest_test

// Focused round-trip tests. Broad (pair × scenario × streaming) coverage
// lives in the harness matrix (cli/harness matrix, CI: harness-matrix.yml);
// this file keeps only cases the matrix does NOT cover: pairs outside
// DefaultPairs, error-status semantics, and the Codex assembly branch.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// OpenAIChat → AnthropicV1 is not in DefaultPairs (the matrix routes Chat to
// the Anthropic *beta* endpoint), so the v1-target conversion is guarded here.
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

// AnthropicV1 → AnthropicV1 is not in DefaultPairs (the matrix's V1
// passthrough targets the beta endpoint), so v1-to-v1 thinking passthrough
// is guarded here.
func TestRoundTrip_AnthropicV1_To_AnthropicV1_Thinking(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pt.ThinkingScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, pt.ThinkingScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ThinkingContent)
	assert.NotEmpty(t, result.Content)
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
// (The matrix also covers this cell; it stays here as the documented
// anchor for the prime-failure pair below.)
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
// real error status with a JSON body, not a 200 with a malformed SSE
// stream that includes an upstream error event.
func TestRoundTrip_StreamingPrimeFailure_To_OpenAIResponses(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.ErrorScenario(), true)

	// The HTTP status must reflect the upstream failure rather than
	// silently 200-with-error-event. SendStreamingError propagates the
	// upstream status (ErrorScenario emits 429) instead of flattening it
	// into a 500; if either side flips to 200 the buffered writer's
	// promotion logic broke.
	assert.Equal(t, 429, result.HTTPStatus,
		"pre-stream prime failure must surface the upstream 429, not a 200 SSE")
	// Parsed assistant content should be empty — no real upstream
	// content ever streamed, so the handler had nothing to convert.
	assert.Empty(t, result.Content,
		"no assistant content should be assembled from a prime-failed stream")
}

// Anthropic-native passthrough: client and provider both speak Anthropic,
// so the request flows through HandleAnthropicBeta (ProcessStream over the
// Anthropic SDK stream). A pre-content upstream error must surface with the
// upstream's retryable status, not a 200 SSE error event — the in-line
// !Written guard in the passthrough converter + ProcessStream's
// no-empty-flush. This is the common multi-Anthropic-account failover shape.
func TestRoundTrip_StreamingPreContentFailure_AnthropicNative(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupRoute(protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, pt.ErrorScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, pt.ErrorScenario(), true)

	assert.Equal(t, 429, result.HTTPStatus,
		"Anthropic-native pre-content failure must surface the upstream 429, not a 200 SSE")
	assert.Empty(t, result.Content,
		"no assistant content should be assembled from a failed pre-content stream")
}

// ---- Codex assembly: nonstream client / stream upstream / assemble ----
// Codex only speaks the streaming Responses API, so a non-streaming client
// request against it is folded into a single message via
// SetupCodexAssemblyRoute (see its doc comment for how this cell of the
// dispatch matrix becomes reachable).

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
// beta source (assembleResponsesToAnthropicBeta instead of the v1 variant).
func TestRoundTrip_CodexAssembly_Beta(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicBeta, pt.StreamingTextScenario())

	result := env.SendAs(t, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, pt.StreamingTextScenario(), false)

	require.Equal(t, 200, result.HTTPStatus)
	assert.Contains(t, result.Content, "Paris")
}

// TestRoundTrip_CodexAssembly_PrimeFailure is TestRoundTrip_StreamingPrimeFailure_To_OpenAIResponses's
// counterpart for the assembly branch: a non-streaming client must still get
// a JSON error with the upstream's status, not a 200 or an SSE frame.
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

// TestRoundTrip_CodexAssembly_NoContentBlocks reproduces #1316's repro end
// to end via ErrorMidStreamCloseScenario, whose stream is cut before any
// content block completes: a retryable error, not 200 with content:null.
func TestRoundTrip_CodexAssembly_NoContentBlocks(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()

	env.SetupCodexAssemblyRoute(protocol.TypeAnthropicV1, pt.ErrorMidStreamCloseScenario())

	result := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, pt.ErrorMidStreamCloseScenario(), false)

	assert.GreaterOrEqual(t, result.HTTPStatus, 400,
		"a stream cut before any content block completes must fail, not return 200 with content:null")
}
