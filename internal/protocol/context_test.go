package protocol

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHandleContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")

	assert.NotNil(t, hc)
	assert.Equal(t, c, hc.GinContext)
	assert.Equal(t, "test-model", hc.ResponseModel)
	assert.Empty(t, hc.OnStreamEventHooks)
	assert.Empty(t, hc.OnStreamCompleteHooks)
	assert.Empty(t, hc.OnStreamErrorHooks)
}

func TestHandleContext_WithOnStreamEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	eventCalled := false
	hc.WithOnStreamEvent(func(event interface{}) error {
		eventCalled = true
		return nil
	})

	assert.Len(t, hc.OnStreamEventHooks, 1)

	// Test calling the hook
	err := hc.OnStreamEventHooks[0](map[string]string{"test": "data"})
	assert.NoError(t, err)
	assert.True(t, eventCalled)
}

func TestHandleContext_WithOnStreamEvent_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	expectedErr := errors.New("hook error")
	hc.WithOnStreamEvent(func(event interface{}) error {
		return expectedErr
	})

	err := hc.OnStreamEventHooks[0](nil)
	assert.Equal(t, expectedErr, err)
}

func TestHandleContext_WithOnStreamComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	completeCalled := false
	hc.WithOnStreamComplete(func() {
		completeCalled = true
	})

	assert.Len(t, hc.OnStreamCompleteHooks, 1)

	// Test calling the hook
	hc.OnStreamCompleteHooks[0]()
	assert.True(t, completeCalled)
}

func TestHandleContext_WithOnStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	expectedErr := errors.New("stream error")
	var receivedErr error
	hc.WithOnStreamError(func(err error) {
		receivedErr = err
	})

	assert.Len(t, hc.OnStreamErrorHooks, 1)

	// Test calling the hook
	hc.OnStreamErrorHooks[0](expectedErr)
	assert.Equal(t, expectedErr, receivedErr)
}

func TestHandleContext_Chaining(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	callOrder := []int{}

	// Chain multiple hooks
	hc.WithOnStreamEvent(func(event interface{}) error {
		callOrder = append(callOrder, 1)
		return nil
	}).WithOnStreamEvent(func(event interface{}) error {
		callOrder = append(callOrder, 2)
		return nil
	}).WithOnStreamComplete(func() {
		callOrder = append(callOrder, 3)
	}).WithOnStreamError(func(err error) {
		callOrder = append(callOrder, 4)
	})

	assert.Len(t, hc.OnStreamEventHooks, 2)
	assert.Len(t, hc.OnStreamCompleteHooks, 1)
	assert.Len(t, hc.OnStreamErrorHooks, 1)

	// Execute in order
	for _, hook := range hc.OnStreamEventHooks {
		hook(nil)
	}
	hc.OnStreamCompleteHooks[0]()
	hc.OnStreamErrorHooks[0](nil)

	assert.Equal(t, []int{1, 2, 3, 4}, callOrder)
}

func TestSetupSSEHeaders(t *testing.T) {
	tests := []struct {
		name          string
		checkHeader   func(*testing.T, http.Header)
	}{
		{
			name: "sets correct SSE headers",
			checkHeader: func(t *testing.T, h http.Header) {
				assert.Equal(t, "text/event-stream; charset=utf-8", h.Get("Content-Type"))
				assert.Equal(t, "no-cache", h.Get("Cache-Control"))
				assert.Equal(t, "keep-alive", h.Get("Connection"))
				assert.Equal(t, "*", h.Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "Cache-Control", h.Get("Access-Control-Allow-Headers"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			hc := NewHandleContext(c, "test-model")
			hc.SetupSSEHeaders()

			tt.checkHeader(t, c.Writer.Header())
		})
	}
}

func TestProcessStream_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	eventsProcessed := []int{}
	completeCalled := false

	hc.WithOnStreamEvent(func(event interface{}) error {
		if num, ok := event.(int); ok {
			eventsProcessed = append(eventsProcessed, num)
		}
		return nil
	}).WithOnStreamComplete(func() {
		completeCalled = true
	})

	// Directly test the hook mechanism without ProcessStream
	// ProcessStream requires http.CloseNotifier which httptest doesn't support
	eventData := []interface{}{1, 2, 3}
	for _, event := range eventData {
		for _, hook := range hc.OnStreamEventHooks {
			if err := hook(event); err != nil {
				t.Errorf("hook error: %v", err)
			}
		}
		// Simulate handleFunc processing
		eventsProcessed = append(eventsProcessed, event.(int))
	}

	// Simulate complete hooks
	hc.CallOnStreamComplete()

	assert.Equal(t, []int{1, 1, 2, 2, 3, 3}, eventsProcessed)
	assert.True(t, completeCalled)
}

func TestProcessStream_HookError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	hookErr := errors.New("hook error")

	hc.WithOnStreamEvent(func(event interface{}) error {
		return hookErr
	})

	// Test that hook error is propagated
	err := hc.OnStreamEventHooks[0](nil)
	assert.Equal(t, hookErr, err)
}

func TestCallOnStreamComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	completeCalled := false
	calledTimes := []int{}

	hc.WithOnStreamComplete(func() {
		completeCalled = true
		calledTimes = append(calledTimes, 1)
	}).WithOnStreamComplete(func() {
		calledTimes = append(calledTimes, 2)
	})

	hc.CallOnStreamComplete()

	assert.True(t, completeCalled)
	assert.Equal(t, []int{1, 2}, calledTimes)
}

func TestSendError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	testErr := errors.New("test error")

	hc.SendError(testErr, "test_type", "test_code")

	// Verify response was sent
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "test error")
}

func TestIsContextCanceled(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		err := context.Canceled
		assert.True(t, IsContextCanceled(err))
	})

	t.Run("wrapped canceled context with %w", func(t *testing.T) {
		// errors.Is only matches when wrapped with %w
		err := fmt.Errorf("wrapped: %w", context.Canceled)
		assert.True(t, IsContextCanceled(err))
	})

	t.Run("other error", func(t *testing.T) {
		err := errors.New("some other error")
		assert.False(t, IsContextCanceled(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsContextCanceled(nil))
	})
}

// ---- Tier 1: stream assembler / OnStreamAssembled tests ----
//
// These tests exercise the new ProcessStream path that feeds typed v1 /
// v1beta events into HandleContext.streamAssembler and delivers the
// assembled *anthropic.Message via OnStreamAssembled hooks.

// streamableRecorder wraps httptest.ResponseRecorder so gin.Context.Stream's
// CloseNotify and Flush calls (which it makes inside ProcessStream) don't
// panic. httptest.ResponseRecorder implements Flusher but NOT CloseNotifier.
type streamableRecorder struct {
	*httptest.ResponseRecorder
	closeCh chan bool
}

func newStreamableRecorder() *streamableRecorder {
	return &streamableRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeCh:          make(chan bool, 1),
	}
}

// CloseNotify implements http.CloseNotifier. The returned channel never
// fires for tests — we drive the stream to completion ourselves.
func (s *streamableRecorder) CloseNotify() <-chan bool { return s.closeCh }

func newTestHandleContext(t *testing.T, model string) (*HandleContext, *streamableRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := newStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return NewHandleContext(c, model), w
}

// driveStream queues `events` through hc.ProcessStream with a no-op
// handleFunc. Returns the error from ProcessStream.
func driveStream(hc *HandleContext, events []interface{}) error {
	i := 0
	return hc.ProcessStream(
		func() (bool, error, interface{}) {
			if i >= len(events) {
				return false, nil, nil
			}
			ev := events[i]
			i++
			return true, nil, ev
		},
		func(_ interface{}) error { return nil },
	)
}

func v1MessageStart(id string) *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{
		Type:    "message_start",
		Message: anthropic.Message{ID: id, Type: "message", Role: "assistant"},
	}
}

func v1ContentBlockStartText(index int) *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{
		Type:         "content_block_start",
		Index:        int64(index),
		ContentBlock: anthropic.ContentBlockStartEventContentBlockUnion{Type: "text"},
	}
}

func v1TextDelta(index int, text string) *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{
		Type:  "content_block_delta",
		Index: int64(index),
		Delta: anthropic.MessageStreamEventUnionDelta{Type: "text_delta", Text: text},
	}
}

func v1ContentBlockStop(index int) *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{Type: "content_block_stop", Index: int64(index)}
}

func v1MessageDelta(stopReason string, in, out int64) *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{
		Type:  "message_delta",
		Delta: anthropic.MessageStreamEventUnionDelta{StopReason: anthropic.StopReason(stopReason)},
		Usage: anthropic.MessageDeltaUsage{InputTokens: in, OutputTokens: out},
	}
}

func v1MessageStop() *anthropic.MessageStreamEventUnion {
	return &anthropic.MessageStreamEventUnion{Type: "message_stop"}
}

func TestProcessStream_AssemblesV1Events(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	var assembled *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) { assembled = msg })

	events := []interface{}{
		v1MessageStart("msg_v1"),
		v1ContentBlockStartText(0),
		v1TextDelta(0, "Hello, "),
		v1TextDelta(0, "world!"),
		v1ContentBlockStop(0),
		v1MessageDelta("end_turn", 11, 7),
		v1MessageStop(),
	}
	require.NoError(t, driveStream(hc, events))

	require.NotNil(t, assembled)
	assert.Equal(t, "msg_v1", assembled.ID)
	assert.Equal(t, "assistant", string(assembled.Role))
	assert.Equal(t, "end_turn", string(assembled.StopReason))
	require.Len(t, assembled.Content, 1)
	assert.Equal(t, "Hello, world!", assembled.Content[0].Text)
}

func betaMessageStart(id string) *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{
		Type:    "message_start",
		Message: anthropic.BetaMessage{ID: id, Type: "message", Role: "assistant"},
	}
}

func betaContentBlockStartText(index int) *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{
		Type:         "content_block_start",
		Index:        int64(index),
		ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"},
	}
}

func betaTextDelta(index int, text string) *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{
		Type:  "content_block_delta",
		Index: int64(index),
		Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: text},
	}
}

func betaContentBlockStop(index int) *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{Type: "content_block_stop", Index: int64(index)}
}

func betaMessageDelta(stopReason string, in, out int64) *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{
		Type:  "message_delta",
		Delta: anthropic.BetaRawMessageStreamEventUnionDelta{StopReason: anthropic.BetaStopReason(stopReason)},
		Usage: anthropic.BetaMessageDeltaUsage{InputTokens: in, OutputTokens: out},
	}
}

func betaMessageStop() *anthropic.BetaRawMessageStreamEventUnion {
	return &anthropic.BetaRawMessageStreamEventUnion{Type: "message_stop"}
}

func TestProcessStream_AssemblesV1BetaEvents(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	var assembled *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) { assembled = msg })

	events := []interface{}{
		betaMessageStart("msg_beta"),
		betaContentBlockStartText(0),
		betaTextDelta(0, "hi "),
		betaTextDelta(0, "there"),
		betaContentBlockStop(0),
		betaMessageDelta("end_turn", 5, 9),
		betaMessageStop(),
	}
	require.NoError(t, driveStream(hc, events))

	require.NotNil(t, assembled)
	assert.Equal(t, "msg_beta", assembled.ID)
	assert.Equal(t, "end_turn", string(assembled.StopReason))
	require.Len(t, assembled.Content, 1)
	assert.Equal(t, "hi there", assembled.Content[0].Text)
}

func TestProcessStream_AssembledFiresBeforeComplete(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")

	order := []string{}
	hc.WithOnStreamEvent(func(_ interface{}) error {
		order = append(order, "event")
		return nil
	})
	hc.WithOnStreamAssembled(func(_ *anthropic.Message) {
		order = append(order, "assembled")
	})
	hc.WithOnStreamComplete(func() {
		order = append(order, "complete")
	})

	events := []interface{}{
		v1MessageStart("msg_order"),
		v1MessageStop(),
	}
	require.NoError(t, driveStream(hc, events))

	// All per-event hooks must run before assembled; assembled before complete.
	require.Equal(t, []string{"event", "event", "assembled", "complete"}, order)
}

func TestProcessStream_UsagePropagation(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	var assembled *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) { assembled = msg })

	events := []interface{}{
		v1MessageStart("msg_usage"),
		v1ContentBlockStartText(0),
		v1TextDelta(0, "x"),
		v1ContentBlockStop(0),
		v1MessageDelta("end_turn", 11, 42),
		v1MessageStop(),
	}
	require.NoError(t, driveStream(hc, events))

	// ProcessStream calls Finish(hc.ResponseModel, 0, 0); the assembler must
	// have captured usage from message_delta internally and emitted it.
	require.NotNil(t, assembled)
	assert.Equal(t, int64(11), assembled.Usage.InputTokens)
	assert.Equal(t, int64(42), assembled.Usage.OutputTokens)
}

func TestProcessStream_NoAssemblerWhenHookAbsent(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	// No WithOnStreamAssembled call — assembler must stay nil.

	events := []interface{}{v1MessageStart("msg_x"), v1MessageStop()}
	require.NoError(t, driveStream(hc, events))

	assert.Nil(t, hc.streamAssembler, "streamAssembler should remain nil when no hook is registered")
	assert.Empty(t, hc.OnStreamAssembledHooks)
}

func TestProcessStream_NoAssemblyOnError(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")

	assembledCalled := false
	errSeen := error(nil)
	hc.WithOnStreamAssembled(func(_ *anthropic.Message) { assembledCalled = true })
	hc.WithOnStreamError(func(err error) { errSeen = err })

	streamErr := errors.New("boom")
	i := 0
	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if i == 0 {
				i++
				return true, nil, v1MessageStart("msg_e")
			}
			return false, streamErr, nil
		},
		func(_ interface{}) error { return nil },
	)
	assert.ErrorIs(t, err, streamErr)
	assert.False(t, assembledCalled, "assembled hook must not fire on error path")
	assert.ErrorIs(t, errSeen, streamErr)
}

func TestProcessStream_AssembledHookGetsNilOnEmptyStream(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	var sawCall bool
	var got *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) {
		sawCall = true
		got = msg
	})

	// No events at all — assembler.Finish returns nil because msgID is empty.
	require.NoError(t, driveStream(hc, nil))

	assert.True(t, sawCall, "assembled hook should fire even for empty streams")
	assert.Nil(t, got, "assembled message should be nil when no message_start was seen")
}

func TestProcessStream_IgnoresUnknownEventTypes(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	var assembled *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) { assembled = msg })

	events := []interface{}{
		v1MessageStart("msg_unk"),
		map[string]interface{}{"type": "ignored_random_event"}, // not a typed union
		"string-event-also-ignored",                            //nolint
		v1ContentBlockStartText(0),
		v1TextDelta(0, "ok"),
		v1ContentBlockStop(0),
		v1MessageStop(),
	}
	require.NoError(t, driveStream(hc, events))

	require.NotNil(t, assembled)
	assert.Equal(t, "msg_unk", assembled.ID)
	require.Len(t, assembled.Content, 1)
	assert.Equal(t, "ok", assembled.Content[0].Text)
}

func TestProcessStream_EventHookErrorSkipsAssembly(t *testing.T) {
	hc, _ := newTestHandleContext(t, "resp-model")
	hookErr := errors.New("hook nope")

	hookCount := 0
	hc.WithOnStreamEvent(func(_ interface{}) error {
		hookCount++
		if hookCount == 2 {
			return hookErr
		}
		return nil
	})

	assembledCalled := false
	hc.WithOnStreamAssembled(func(_ *anthropic.Message) { assembledCalled = true })

	events := []interface{}{
		v1MessageStart("msg_a"),
		v1TextDelta(0, "X"), // 2nd event — hook errors here
		v1MessageStop(),
	}
	err := driveStream(hc, events)
	assert.ErrorIs(t, err, hookErr)

	// Assembled hooks must not fire when ProcessStream short-circuits on error.
	assert.False(t, assembledCalled)

	// hc.streamAssembler is allocated lazily by WithOnStreamAssembled, so it
	// exists — but the second event must NOT have been fed into it. We can't
	// peek the assembler's blocks directly, so assert behaviourally via the
	// short-circuit: hookCount stopped at 2.
	assert.Equal(t, 2, hookCount)
}

func TestProcessStream_AssembledGolden(t *testing.T) {
	hc, _ := newTestHandleContext(t, "golden-model")
	var assembled *anthropic.Message
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) { assembled = msg })

	events := []interface{}{
		v1MessageStart("msg_golden"),
		v1ContentBlockStartText(0),
		v1TextDelta(0, "alpha "),
		v1TextDelta(0, "beta "),
		v1TextDelta(0, "gamma"),
		v1ContentBlockStop(0),
		v1MessageDelta("end_turn", 13, 25),
		v1MessageStop(),
	}
	require.NoError(t, driveStream(hc, events))
	require.NotNil(t, assembled)

	// Compare via JSON to sidestep the SDK's unexported `respjson.Field`
	// metadata, which cmp.Diff cannot traverse. The recorded body that
	// reaches obs.Sink is always a map (coerceToMap json-marshals the
	// message), so JSON equivalence is the contract that actually matters.
	gotJSON, err := json.Marshal(assembled)
	require.NoError(t, err)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(gotJSON, &got))

	// Locks down: id, role, stop_reason, content text, usage propagation
	// through Finish(model, 0, 0). Model comes from hc.ResponseModel.
	assert.Equal(t, "msg_golden", got["id"])
	assert.Equal(t, "message", got["type"])
	assert.Equal(t, "assistant", got["role"])
	assert.Equal(t, "golden-model", got["model"])
	assert.Equal(t, "end_turn", got["stop_reason"])

	content, ok := got["content"].([]interface{})
	require.True(t, ok, "content must be an array")
	require.Len(t, content, 1)
	block, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "text", block["type"])
	assert.Equal(t, "alpha beta gamma", block["text"])

	usage, ok := got["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 13, usage["input_tokens"])
	assert.EqualValues(t, 25, usage["output_tokens"], "usage must propagate through Finish(model, 0, 0)")
}

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		errorType string
		code     string
	}{
		{
			name:     "basic error response",
			message:  "Something went wrong",
			errorType: "api_error",
			code:     "invalid_request",
		},
		{
			name:     "error without code",
			message:  "Another error",
			errorType: "validation_error",
			code:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := ErrorResponse{
				Error: ErrorDetail{
					Message: tt.message,
					Type:    tt.errorType,
					Code:    tt.code,
				},
			}

			// Verify structure
			assert.Equal(t, tt.message, resp.Error.Message)
			assert.Equal(t, tt.errorType, resp.Error.Type)
			assert.Equal(t, tt.code, resp.Error.Code)
		})
	}
}
