package streamemit

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// feedV1Raw is a small helper that unmarshals a raw v1 event JSON and
// feeds it into the emitter, returning the released events.
func feedV1Raw(t *testing.T, e *StreamEmitter, raw string) []BufferedEvent {
	t.Helper()
	out, err := e.FeedV1(v1Event(t, raw))
	require.NoError(t, err)
	return out
}

func feedV1BetaRaw(t *testing.T, e *StreamEmitter, raw string) []BufferedEvent {
	t.Helper()
	out, err := e.FeedV1Beta(betaEvent(t, raw))
	require.NoError(t, err)
	return out
}

func TestStreamEmitter_TextOnly_EmitImmediate_v1(t *testing.T) {
	e := New(Config{})

	got := feedV1Raw(t, e, fxMessageStart("msg_1"))
	require.Len(t, got, 1)
	assert.Equal(t, "message_start", got[0].EventType)

	got = feedV1Raw(t, e, fxTextBlockStart(0))
	require.Len(t, got, 1)
	assert.Equal(t, "content_block_start", got[0].EventType)

	got = feedV1Raw(t, e, fxTextDelta(0, "He"))
	require.Len(t, got, 1)
	assert.Equal(t, "content_block_delta", got[0].EventType)

	got = feedV1Raw(t, e, fxTextDelta(0, "llo"))
	require.Len(t, got, 1)

	got = feedV1Raw(t, e, fxBlockStop(0))
	require.Len(t, got, 1)
	assert.Equal(t, "content_block_stop", got[0].EventType)

	got = feedV1Raw(t, e, fxMessageDelta("end_turn", 5))
	require.Len(t, got, 1)
	got = feedV1Raw(t, e, fxMessageStop())
	require.Len(t, got, 1)

	// No tool buffer was ever opened.
	_, hasBuf := e.ToolBuffer(0)
	assert.False(t, hasBuf)

	pending, msg := e.Finish("claude-test", 1, 5)
	assert.Empty(t, pending)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "text", string(msg.Content[0].Type))
	assert.Equal(t, "Hello", msg.Content[0].Text)
}

func TestStreamEmitter_ToolOnly_EmitOnComplete_v1(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})

	got := feedV1Raw(t, e, fxMessageStart("msg_2"))
	require.Len(t, got, 1)

	// Tool block start: should be buffered.
	got = feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_1", "get_weather"))
	assert.Empty(t, got)
	snap, ok := e.ToolBuffer(0)
	require.True(t, ok)
	require.Len(t, snap, 1)

	// Three input_json_delta events: still buffered.
	got = feedV1Raw(t, e, fxInputJSONDelta(0, `{"ci`))
	assert.Empty(t, got)
	got = feedV1Raw(t, e, fxInputJSONDelta(0, `ty":"S`))
	assert.Empty(t, got)
	got = feedV1Raw(t, e, fxInputJSONDelta(0, `F"}`))
	assert.Empty(t, got)

	// content_block_stop flushes start + 3 deltas + stop in order.
	got = feedV1Raw(t, e, fxBlockStop(0))
	require.Len(t, got, 5)
	assert.Equal(t, "content_block_start", got[0].EventType)
	assert.Equal(t, "content_block_delta", got[1].EventType)
	assert.Equal(t, "content_block_delta", got[2].EventType)
	assert.Equal(t, "content_block_delta", got[3].EventType)
	assert.Equal(t, "content_block_stop", got[4].EventType)

	// Buffer is gone after flush.
	_, ok = e.ToolBuffer(0)
	assert.False(t, ok)

	feedV1Raw(t, e, fxMessageDelta("tool_use", 7))
	feedV1Raw(t, e, fxMessageStop())

	pending, msg := e.Finish("claude-test", 1, 7)
	assert.Empty(t, pending)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "tool_use", string(msg.Content[0].Type))
	assert.Equal(t, "toolu_1", msg.Content[0].ID)
	assert.Equal(t, "get_weather", msg.Content[0].Name)
	assert.JSONEq(t, `{"city":"SF"}`, string(msg.Content[0].Input))
}

func TestStreamEmitter_Mixed_TextLive_ToolBuffered_v1(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})

	feedV1Raw(t, e, fxMessageStart("msg_3"))

	// Block 0: text — flows live.
	got := feedV1Raw(t, e, fxTextBlockStart(0))
	require.Len(t, got, 1)
	got = feedV1Raw(t, e, fxTextDelta(0, "hi"))
	require.Len(t, got, 1)
	got = feedV1Raw(t, e, fxBlockStop(0))
	require.Len(t, got, 1)

	// Block 1: tool_use — buffered.
	got = feedV1Raw(t, e, fxToolUseBlockStart(1, "toolu_2", "noop"))
	assert.Empty(t, got)
	got = feedV1Raw(t, e, fxInputJSONDelta(1, `{`))
	assert.Empty(t, got)
	got = feedV1Raw(t, e, fxInputJSONDelta(1, `}`))
	assert.Empty(t, got)
	got = feedV1Raw(t, e, fxBlockStop(1))
	require.Len(t, got, 4) // start + 2 deltas + stop
	for _, ev := range got {
		assert.Equal(t, 1.0, ev.Payload["index"])
	}

	feedV1Raw(t, e, fxMessageDelta("tool_use", 9))
	feedV1Raw(t, e, fxMessageStop())

	_, msg := e.Finish("claude-test", 1, 9)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, "text", string(msg.Content[0].Type))
	assert.Equal(t, "hi", msg.Content[0].Text)
	assert.Equal(t, "tool_use", string(msg.Content[1].Type))
	assert.JSONEq(t, `{}`, string(msg.Content[1].Input))
}

func TestStreamEmitter_Mixed_PendingTextDeltaThenToolStop_v1(t *testing.T) {
	// Open text block 0, open tool block 1, more text deltas, then stop tool 1.
	// Verify the tool flush returns ONLY tool events; none of the text events
	// already emitted earlier leak into the tool buffer's drain.
	e := New(Config{ToolPolicy: EmitOnComplete})

	feedV1Raw(t, e, fxMessageStart("msg_4"))
	feedV1Raw(t, e, fxTextBlockStart(0))
	feedV1Raw(t, e, fxTextDelta(0, "a"))

	// Tool 1 starts and is buffered.
	got := feedV1Raw(t, e, fxToolUseBlockStart(1, "toolu_3", "f"))
	assert.Empty(t, got)

	// More text on block 0 keeps flowing live.
	got = feedV1Raw(t, e, fxTextDelta(0, "b"))
	require.Len(t, got, 1)
	assert.Equal(t, "content_block_delta", got[0].EventType)
	assert.Equal(t, 0.0, got[0].Payload["index"])

	got = feedV1Raw(t, e, fxInputJSONDelta(1, `{}`))
	assert.Empty(t, got)

	// Stop tool 1: flush should contain only the tool's events (start, json
	// delta, stop) — nothing from block 0.
	got = feedV1Raw(t, e, fxBlockStop(1))
	require.Len(t, got, 3)
	for _, ev := range got {
		assert.Equal(t, 1.0, ev.Payload["index"], "expected only tool block events")
	}
}

func TestStreamEmitter_TextOnly_EmitImmediate_v1Beta(t *testing.T) {
	e := New(Config{})
	got := feedV1BetaRaw(t, e, fxMessageStart("msg_b1"))
	require.Len(t, got, 1)

	got = feedV1BetaRaw(t, e, fxTextBlockStart(0))
	require.Len(t, got, 1)
	got = feedV1BetaRaw(t, e, fxTextDelta(0, "Hi"))
	require.Len(t, got, 1)
	got = feedV1BetaRaw(t, e, fxBlockStop(0))
	require.Len(t, got, 1)
	feedV1BetaRaw(t, e, fxMessageDelta("end_turn", 2))
	feedV1BetaRaw(t, e, fxMessageStop())

	pending, msg := e.Finish("claude-test", 1, 2)
	assert.Empty(t, pending)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "Hi", msg.Content[0].Text)
}

func TestStreamEmitter_ToolOnly_EmitOnComplete_v1Beta(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})
	feedV1BetaRaw(t, e, fxMessageStart("msg_b2"))

	got := feedV1BetaRaw(t, e, fxToolUseBlockStart(0, "toolu_b1", "f"))
	assert.Empty(t, got)
	feedV1BetaRaw(t, e, fxInputJSONDelta(0, `{"k":1}`))
	got = feedV1BetaRaw(t, e, fxBlockStop(0))
	require.Len(t, got, 3)

	feedV1BetaRaw(t, e, fxMessageDelta("tool_use", 3))
	feedV1BetaRaw(t, e, fxMessageStop())

	_, msg := e.Finish("claude-test", 1, 3)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "tool_use", string(msg.Content[0].Type))
	assert.JSONEq(t, `{"k":1}`, string(msg.Content[0].Input))
}

func TestStreamEmitter_Mixed_v1Beta(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})
	feedV1BetaRaw(t, e, fxMessageStart("msg_b3"))

	got := feedV1BetaRaw(t, e, fxTextBlockStart(0))
	require.Len(t, got, 1)
	feedV1BetaRaw(t, e, fxTextDelta(0, "x"))
	feedV1BetaRaw(t, e, fxBlockStop(0))

	got = feedV1BetaRaw(t, e, fxToolUseBlockStart(1, "toolu_b2", "g"))
	assert.Empty(t, got)
	feedV1BetaRaw(t, e, fxInputJSONDelta(1, `{}`))
	got = feedV1BetaRaw(t, e, fxBlockStop(1))
	require.Len(t, got, 3)

	_, msg := e.Finish("claude-test", 1, 4)
	require.NotNil(t, msg)
	require.Len(t, msg.Content, 2)
}

func TestStreamEmitter_Drain_FlushesUnclosedToolBlock(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})
	feedV1Raw(t, e, fxMessageStart("msg_5"))
	feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_x", "f"))
	feedV1Raw(t, e, fxInputJSONDelta(0, `{`))
	feedV1Raw(t, e, fxInputJSONDelta(0, `}`))

	pending := e.Drain()
	require.Len(t, pending, 3) // start + 2 deltas, no stop

	// Buffer is now empty.
	_, ok := e.ToolBuffer(0)
	assert.False(t, ok)

	// A second drain returns nothing.
	assert.Empty(t, e.Drain())
}

func TestStreamEmitter_Finish_ReturnsPendingAndMessage(t *testing.T) {
	e := New(Config{ToolPolicy: EmitOnComplete})
	feedV1Raw(t, e, fxMessageStart("msg_6"))
	feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_y", "f"))
	feedV1Raw(t, e, fxInputJSONDelta(0, `{}`))

	pending, msg := e.Finish("claude-test", 1, 1)
	require.Len(t, pending, 2) // start + 1 delta, drained by Finish
	require.NotNil(t, msg)
	// Inner assembler still produced a tool_use block reflecting what it saw.
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "tool_use", string(msg.Content[0].Type))
}

func TestStreamEmitter_OnToolBlockComplete_ReplacesBuffered(t *testing.T) {
	synthetic := []BufferedEvent{
		{EventType: "content_block_start", Payload: map[string]interface{}{"type": "content_block_start", "index": 0, "content_block": map[string]interface{}{"type": "text", "text": ""}}},
		{EventType: "content_block_delta", Payload: map[string]interface{}{"type": "content_block_delta", "index": 0, "delta": map[string]interface{}{"type": "text_delta", "text": "blocked"}}},
		{EventType: "content_block_stop", Payload: map[string]interface{}{"type": "content_block_stop", "index": 0}},
	}

	var sawID string
	e := New(Config{
		ToolPolicy: EmitOnComplete,
		OnToolBlockComplete: func(toolID string, index int, buffered []BufferedEvent) (*ToolDecision, error) {
			sawID = toolID
			return &ToolDecision{Replace: synthetic}, nil
		},
	})

	feedV1Raw(t, e, fxMessageStart("msg_7"))
	feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_h", "f"))
	feedV1Raw(t, e, fxInputJSONDelta(0, `{}`))
	got := feedV1Raw(t, e, fxBlockStop(0))

	assert.Equal(t, "toolu_h", sawID)
	require.Len(t, got, 3)
	assert.Equal(t, "content_block_delta", got[1].EventType)
	delta := got[1].Payload["delta"].(map[string]interface{})
	assert.Equal(t, "blocked", delta["text"])
}

func TestStreamEmitter_OnToolBlockComplete_DropDrops(t *testing.T) {
	e := New(Config{
		ToolPolicy: EmitOnComplete,
		OnToolBlockComplete: func(toolID string, index int, buffered []BufferedEvent) (*ToolDecision, error) {
			return &ToolDecision{Drop: true}, nil
		},
	})

	feedV1Raw(t, e, fxMessageStart("msg_8"))
	feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_d", "f"))
	feedV1Raw(t, e, fxInputJSONDelta(0, `{}`))
	got := feedV1Raw(t, e, fxBlockStop(0))
	assert.Empty(t, got)
}

func TestStreamEmitter_OnToolBlockComplete_ErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	e := New(Config{
		ToolPolicy: EmitOnComplete,
		OnToolBlockComplete: func(toolID string, index int, buffered []BufferedEvent) (*ToolDecision, error) {
			return nil, wantErr
		},
	})

	_, err := e.FeedV1(v1Event(t, fxMessageStart("msg_e")))
	require.NoError(t, err)
	_, err = e.FeedV1(v1Event(t, fxToolUseBlockStart(0, "t", "f")))
	require.NoError(t, err)
	_, err = e.FeedV1(v1Event(t, fxBlockStop(0)))
	assert.ErrorIs(t, err, wantErr)
}

func TestStreamEmitter_RejectsMixedVersions(t *testing.T) {
	e := New(Config{})
	_, err := e.FeedV1(v1Event(t, fxMessageStart("msg_v1")))
	require.NoError(t, err)

	_, err = e.FeedV1Beta(betaEvent(t, fxMessageStart("msg_beta")))
	assert.ErrorIs(t, err, ErrMixedVersions)
}

func TestStreamEmitter_GuardrailsCompatSketch(t *testing.T) {
	// The output of streamemit is byte-compatible with
	// protocol.GuardrailsBufferedEvent (it's the same type via alias).
	// This test demonstrates that the Payload map a guardrails consumer
	// would build by hand matches what the emitter produces.
	e := New(Config{ToolPolicy: EmitOnComplete})
	feedV1Raw(t, e, fxMessageStart("msg_gr"))
	feedV1Raw(t, e, fxToolUseBlockStart(0, "toolu_gr", "f"))
	feedV1Raw(t, e, fxInputJSONDelta(0, `{}`))
	flushed := feedV1Raw(t, e, fxBlockStop(0))

	// Construct an equivalent slice of GuardrailsBufferedEvent from the
	// same raw JSON payloads — the alias makes assignment trivial.
	var guardrailsView []protocol.GuardrailsBufferedEvent = flushed
	require.Len(t, guardrailsView, 3)
	assert.Equal(t, "content_block_start", guardrailsView[0].EventType)

	// And the Payload round-trips through json.Marshal -> json.Unmarshal,
	// which is what sendAnthropicStreamEvent does.
	for _, ev := range guardrailsView {
		raw, err := json.Marshal(ev.Payload)
		require.NoError(t, err)
		var back map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &back))
		assert.Equal(t, ev.Payload["type"], back["type"])
	}
}
