package record

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type countingMarshaler struct {
	calls *int
}

type rawJSONValue struct {
	raw string
}

func (v rawJSONValue) RawJSON() string { return v.raw }

func (rawJSONValue) MarshalJSON() ([]byte, error) {
	return []byte(`{"expanded_zero_value":""}`), nil
}

func (m countingMarshaler) MarshalJSON() ([]byte, error) {
	(*m.calls)++
	return []byte(`{"unexpected":true}`), nil
}

func TestDisabledRecorderDoesNoCaptureWork(t *testing.T) {
	calls := 0
	recorder, err := New(Config{
		Enabled: false,
		Input:   countingMarshaler{calls: &calls},
	})
	require.NoError(t, err)
	require.Nil(t, recorder)
	require.Zero(t, calls)

	exchange, err := recorder.BeginExchange(ExchangeMetadata{}, countingMarshaler{calls: &calls})
	require.NoError(t, err)
	require.Nil(t, exchange)
	require.NoError(t, recorder.SetFinalResponse("", countingMarshaler{calls: &calls}))
	require.NoError(t, exchange.Finish(countingMarshaler{calls: &calls}, nil))
	require.Zero(t, calls)

	completed, first := recorder.Finish(nil)
	require.Nil(t, completed)
	require.False(t, first)
}

func TestRecorderCapturesOrderedProviderExchanges(t *testing.T) {
	recorder, err := New(Config{
		Enabled:       true,
		RequestID:     "req-1",
		SessionID:     "session-1",
		Scenario:      "chat",
		InputProtocol: protocol.TypeOpenAIChat,
		Input:         map[string]any{"model": "public", "messages": []any{}},
	})
	require.NoError(t, err)
	require.True(t, recorder.Enabled())

	first, err := recorder.BeginExchange(ExchangeMetadata{
		Attempt:  1,
		Provider: "primary",
		Model:    "provider-model-a",
		Protocol: protocol.TypeAnthropicBeta,
	}, map[string]any{"model": "provider-model-a"})
	require.NoError(t, err)
	require.NoError(t, first.Finish(nil, errors.New("provider unavailable")))

	second, err := recorder.BeginExchange(ExchangeMetadata{
		Attempt:  2,
		Provider: "fallback",
		Model:    "provider-model-b",
		Protocol: protocol.TypeOpenAIChat,
	}, map[string]any{"model": "provider-model-b"})
	require.NoError(t, err)
	require.NoError(t, second.Finish(map[string]any{"id": "provider-response"}, nil))
	require.NoError(t, recorder.SetFinalResponse(
		protocol.TypeOpenAIChat,
		map[string]any{"id": "client-response", "model": "public"},
	))

	completed, firstFinish := recorder.Finish(nil)
	require.True(t, firstFinish)
	require.Equal(t, "req-1", completed.RequestID)
	require.Equal(t, OutcomeSucceeded, completed.Outcome)
	require.Len(t, completed.ProviderExchanges, 2)
	require.Equal(t, 1, completed.ProviderExchanges[0].Sequence)
	require.Equal(t, 1, completed.ProviderExchanges[0].Attempt)
	require.Equal(t, OutcomeFailed, completed.ProviderExchanges[0].Outcome)
	require.Equal(t, "provider unavailable", completed.ProviderExchanges[0].Error)
	require.Nil(t, completed.ProviderExchanges[0].Response)
	require.Equal(t, 2, completed.ProviderExchanges[1].Sequence)
	require.Equal(t, 2, completed.ProviderExchanges[1].Attempt)
	require.Equal(t, OutcomeSucceeded, completed.ProviderExchanges[1].Outcome)
	require.Equal(t, protocol.TypeOpenAIChat, completed.FinalResponse.Protocol)
	require.JSONEq(t, `{"id":"client-response","model":"public"}`, string(completed.FinalResponse.Body))

	// Finish is idempotent and returns defensive copies.
	completed.InputRequest.Body[0] = 'x'
	again, secondFinish := recorder.Finish(nil)
	require.False(t, secondFinish)
	require.True(t, json.Valid(again.InputRequest.Body))
	require.JSONEq(t, `{"messages":[],"model":"public"}`, string(again.InputRequest.Body))
}

func TestRecorderKeepsToolLoopRoundsInOneAttempt(t *testing.T) {
	recorder, err := New(Config{
		Enabled:       true,
		InputProtocol: protocol.TypeAnthropicBeta,
		Input:         map[string]any{"model": "claude"},
	})
	require.NoError(t, err)

	for round := 1; round <= 3; round++ {
		exchange, beginErr := recorder.BeginExchange(ExchangeMetadata{
			Attempt:  1,
			Provider: "provider",
			Model:    "claude",
			Protocol: protocol.TypeAnthropicBeta,
		}, map[string]any{"round": round})
		require.NoError(t, beginErr)
		require.NoError(t, exchange.Finish(map[string]any{"round": round}, nil))
	}

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 3)
	for i, exchange := range completed.ProviderExchanges {
		require.Equal(t, i+1, exchange.Sequence)
		require.Equal(t, 1, exchange.Attempt)
	}
}

func TestRecorderPrefersProtocolRawJSON(t *testing.T) {
	recorder, err := New(Config{
		Enabled:       true,
		InputProtocol: protocol.TypeAnthropicBeta,
		Input:         map[string]any{"model": "client"},
	})
	require.NoError(t, err)
	exchange, err := recorder.BeginExchange(ExchangeMetadata{
		Protocol: protocol.TypeAnthropicBeta,
	}, map[string]any{"model": "provider"})
	require.NoError(t, err)
	require.NoError(t, exchange.Finish(rawJSONValue{raw: `{"model":"provider","unknown":"preserved"}`}, nil))

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.JSONEq(t,
		`{"model":"provider","unknown":"preserved"}`,
		string(completed.ProviderExchanges[0].Response.Body),
	)
}

func TestRecorderMapsCancellationAndRejectsMutationAfterFinish(t *testing.T) {
	recorder, err := New(Config{
		Enabled:       true,
		InputProtocol: protocol.TypeOpenAIResponses,
		Input:         map[string]any{"model": "gpt"},
	})
	require.NoError(t, err)

	completed, first := recorder.Finish(context.Canceled)
	require.True(t, first)
	require.Equal(t, OutcomeCancelled, completed.Outcome)
	require.ErrorIs(t, recorder.SetFinalResponse(protocol.TypeOpenAIResponses, map[string]any{}), ErrFinished)
	_, err = recorder.BeginExchange(ExchangeMetadata{Protocol: protocol.TypeOpenAIResponses}, map[string]any{})
	require.ErrorIs(t, err, ErrFinished)
}

func TestEnabledRecorderValidatesProtocolAndJSON(t *testing.T) {
	_, err := New(Config{Enabled: true, Input: map[string]any{}})
	require.ErrorContains(t, err, "protocol is empty")

	_, err = New(Config{
		Enabled:       true,
		InputProtocol: protocol.TypeOpenAIChat,
		Input:         func() {},
	})
	require.ErrorContains(t, err, "unsupported type")
}
