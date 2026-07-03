package recording_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/server/recordingtest"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---- Tier 2: AttachRecorderHooks integration with obs.Sink ----
//
// These tests prove that AttachRecorderHooks + ProtocolRecorder + obs.Sink
// together produce the expected record without spinning up HTTP. The seam
// is an in-memory RecordExporter installed via obs.WithExporters.
//
// This file lives in the external recording_test package (not recording)
// because recordingtest.NewRecordingTestHandler needs to build a
// *server.ProtocolHandler, and internal/server imports
// internal/server/recording in production code — a same-package (internal)
// test file importing internal/server here would be a real import cycle.
// The shared test helpers themselves live in internal/server/recordingtest
// (a plain, non-_test.go package) so that internal/server's own e2e test
// (anthropic_recording_e2e_test.go) can reuse them too — Go test files,
// even in an external _test package, are never importable from another
// package.

// driveBetaStream runs a v1beta event sequence through hc.ProcessStream with
// a no-op handleFunc.
func driveBetaStream(hc *protocol.HandleContext, events []*anthropic.BetaRawMessageStreamEventUnion) error {
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

func TestAttachRecorderHooks_HappyPath_V1Beta(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordModeStagedRequestResponse)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{"model":"client-model"}`))

	provider := &typ.Provider{Name: "anthropic-prov"}
	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "actual-model", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "proxy-model")
	recording.AttachRecorderHooks(hc, recorder, "actual-model", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_hp", Type: "message", Role: "assistant"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "hello"}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_delta", Delta: anthropic.BetaRawMessageStreamEventUnionDelta{StopReason: "end_turn"}, Usage: anthropic.BetaMessageDeltaUsage{InputTokens: 4, OutputTokens: 9}},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))

	records := mem.Snapshot()
	require.Len(t, records, 1, "exactly one obs.Record should be emitted")
	rec := records[0]
	require.NotNil(t, rec.FinalResponse, "FinalResponse must be set after successful stream")
	body := rec.FinalResponse.Body
	require.NotNil(t, body)
	assert.Equal(t, "msg_hp", body["id"])
	assert.Equal(t, "actual-model", body["model"], "AttachRecorderHooks must override model to actualModel")
	usage, ok := body["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 9, usage["output_tokens"])
	content, ok := body["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1)
	block, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "hello", block["text"])
}

func TestAttachRecorderHooks_StreamChunksRecorded(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordModeStagedRequestResponse)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "m", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "rm")
	recording.AttachRecorderHooks(hc, recorder, "m", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_chunks"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "a"}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))

	records := mem.Snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].FinalResponse)
	// All chunks must be captured (synthesizeFinalResponse is the path that
	// writes them; happy path uses SetAssembledResponse which drops chunks,
	// so we assert via the FinalResponse.IsStreaming flag instead).
	assert.True(t, records[0].FinalResponse.IsStreaming, "FinalResponse.IsStreaming must be true for a streamed record")
}

func TestAttachRecorderHooks_ModelOverride(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordModeStagedRequestResponse)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "actual-X", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "proxy-Y")
	recording.AttachRecorderHooks(hc, recorder, "actual-X", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_mo"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "."}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))
	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))

	records := mem.Snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].FinalResponse)
	assert.Equal(t, "actual-X", records[0].FinalResponse.Body["model"],
		"AttachRecorderHooks must override msg.Model with the actualModel arg, not hc.ResponseModel")
}

func TestAttachRecorderHooks_ErrorPath(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordModeStagedRequestResponse)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "m", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "rm")
	recording.AttachRecorderHooks(hc, recorder, "m", provider)

	streamErr := errors.New("upstream broke")
	i := 0
	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if i == 0 {
				i++
				return true, nil, &anthropic.BetaRawMessageStreamEventUnion{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_err"}}
			}
			return false, streamErr, nil
		},
		func(_ interface{}) error { return nil },
	)
	assert.ErrorIs(t, err, streamErr)

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))

	records := mem.Snapshot()
	require.Len(t, records, 1, "exactly one error record must be emitted")
	assert.NotEmpty(t, records[0].Err, "record.Err must be populated on stream error")
	assert.Nil(t, records[0].FinalResponse, "no FinalResponse on error path")
}

func TestAttachRecorderHooks_NilRecorder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := recordingtest.NewStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	hc := protocol.NewHandleContext(c, "rm")
	recording.AttachRecorderHooks(hc, nil, "m", &typ.Provider{Name: "p"})

	assert.Empty(t, hc.OnStreamEventHooks)
	assert.Empty(t, hc.OnStreamCompleteHooks)
	assert.Empty(t, hc.OnStreamErrorHooks)

	// And the stream still runs cleanly with no recorder attached.
	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_nil"}},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))
}
