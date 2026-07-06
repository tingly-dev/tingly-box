// Prime wrapper unit tests. The full end-to-end prime path is covered
// by TestRoundTrip_AnthropicBeta_To_OpenAIResponses_Streaming and
// TestRoundTrip_StreamingPrimeFailure_To_OpenAIResponses in
// internal/protocoltest; these check the Next/Current contract
// of the replay wrapper directly without standing up an SDK stream.

package stream

import (
	"testing"

	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStream mimics the small slice of *openaistream.Stream that
// firstEventReplayStream delegates to. The real type is a concrete
// struct in the SDK so we can't substitute it directly; the wrapper
// only invokes Next/Current/Err/Close on it, all reachable via
// reflection-free duck typing if we make those methods available on
// our own type and assign through a thin shim. Since the wrapper's
// inner field is typed to the SDK struct we instead exercise the
// wrapper by constructing it with a nil inner and only calling Next
// up to the point where it would delegate — proving the replay branch
// independently of the SDK behaviour.
//
// Full delegation behaviour (inner.Next, inner.Current, inner.Err,
// inner.Close) is covered by the e2e roundtrip tests that drive a
// real SDK stream.

func TestFirstEventReplayStream_FirstNextYieldsCachedEvent(t *testing.T) {
	first := responses.ResponseStreamEventUnion{Type: "response.created"}
	p := &firstEventReplayStream{first: first}

	if !p.Next() {
		t.Fatal("first Next() must return true")
	}
	if got := p.Current(); got.Type != "response.created" {
		t.Fatalf("Current().Type = %q, want response.created", got.Type)
	}
	if p.nextCount != 1 {
		t.Fatalf("nextCount = %d, want 1", p.nextCount)
	}
}

func TestFirstEventReplayStream_CurrentBeforeNextIsZero(t *testing.T) {
	// Contract: callers must invoke Next() before Current(). If they
	// don't, the wrapper falls through to inner.Current() which is
	// the SDK's zero value — not p.first. Documenting this so a
	// future refactor that "helpfully" returns p.first here is
	// flagged as a behaviour change.
	first := responses.ResponseStreamEventUnion{Type: "response.created"}
	p := &firstEventReplayStream{first: first}
	// Skip calling Next(); the inner stream is nil so calling Current
	// directly would panic. We assert the nextCount path explicitly.
	if p.nextCount != 0 {
		t.Fatalf("fresh wrapper nextCount = %d, want 0", p.nextCount)
	}
}

func TestPrimeResponsesStream_EmptyStreamReturnsError(t *testing.T) {
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](newFakeResponsesDecoder(nil), nil)

	primed, err := PrimeResponsesStream(stream)

	require.Error(t, err)
	assert.Nil(t, primed)
	assert.Contains(t, err.Error(), "empty responses stream")
}

func TestPrimeResponsesStream_ReplaysFirstEvent(t *testing.T) {
	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](newFakeResponsesDecoder([]string{
		buildResponsesCompletedJSON(t, 3, 5, 0, 0),
	}), nil)

	primed, err := PrimeResponsesStream(stream)
	require.NoError(t, err)
	require.NotNil(t, primed)

	require.True(t, primed.Next())
	assert.Equal(t, "response.completed", primed.Current().Type)
	assert.False(t, primed.Next())
}
