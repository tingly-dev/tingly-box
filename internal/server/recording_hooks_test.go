package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---- Tier 2: AttachRecorderHooks integration with obs.Sink ----
//
// These tests prove that AttachRecorderHooks + ProtocolRecorder + obs.Sink
// together produce the expected record without spinning up HTTP. The seam
// is an in-memory RecordExporter installed via obs.WithExporters.

// memExporter captures Records emitted by the BatchProcessor for inspection.
// The mutex is mandatory: BatchProcessor exports on its own goroutine.
type memExporter struct {
	mu      sync.Mutex
	records []*obs.Record
}

func (m *memExporter) Export(_ context.Context, rs []*obs.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, rs...)
	return nil
}

func (m *memExporter) Shutdown(context.Context) error { return nil }

func (m *memExporter) snapshot() []*obs.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*obs.Record, len(m.records))
	copy(out, m.records)
	return out
}

// streamableRecorder adds CloseNotifier to httptest.ResponseRecorder so
// gin.Context.Stream (which ProcessStream calls) doesn't panic.
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

func (s *streamableRecorder) CloseNotify() <-chan bool { return s.closeCh }

func newRecordingTestServer(t *testing.T, scenario typ.RuleScenario, mode obs.RecordMode) (*Server, *memExporter) {
	t.Helper()
	mem := &memExporter{}
	sink := obs.NewSink("", mode, obs.WithExporters(mem))
	require.NotNil(t, sink, "obs.NewSink must succeed with WithExporters")

	s := &Server{
		scenarioRecordSinks: map[typ.RuleScenario]*obs.Sink{scenario: sink},
		recordMode:          mode,
	}
	t.Cleanup(func() { sink.Close() })
	return s, mem
}

func newRecordingTestContext(t *testing.T, body []byte) (*gin.Context, *streamableRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := newStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

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
	s, mem := newRecordingTestServer(t, scenario, obs.RecordModeAll)
	c, _ := newRecordingTestContext(t, []byte(`{"model":"client-model"}`))

	provider := &typ.Provider{Name: "anthropic-prov"}
	recorder := s.EnsureProtocolRecorder(c, string(scenario), provider, "actual-model", obs.RecordModeAll)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "proxy-model")
	AttachRecorderHooks(hc, recorder, "actual-model", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_hp", Type: "message", Role: "assistant"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "hello"}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_delta", Delta: anthropic.BetaRawMessageStreamEventUnionDelta{StopReason: "end_turn"}, Usage: anthropic.BetaMessageDeltaUsage{InputTokens: 4, OutputTokens: 9}},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))

	sink := s.scenarioRecordSinks[scenario]
	require.NoError(t, sink.ForceFlush(ctxWithTimeout(t)))

	records := mem.snapshot()
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
	s, mem := newRecordingTestServer(t, scenario, obs.RecordModeAll)
	c, _ := newRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := s.EnsureProtocolRecorder(c, string(scenario), provider, "m", obs.RecordModeAll)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "rm")
	AttachRecorderHooks(hc, recorder, "m", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_chunks"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "a"}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))

	require.NoError(t, s.scenarioRecordSinks[scenario].ForceFlush(ctxWithTimeout(t)))

	records := mem.snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].FinalResponse)
	// All chunks must be captured (synthesizeFinalResponse is the path that
	// writes them; happy path uses SetAssembledResponse which drops chunks,
	// so we assert via the FinalResponse.IsStreaming flag instead).
	assert.True(t, records[0].FinalResponse.IsStreaming, "FinalResponse.IsStreaming must be true for a streamed record")
}

func TestAttachRecorderHooks_ModelOverride(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	s, mem := newRecordingTestServer(t, scenario, obs.RecordModeAll)
	c, _ := newRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := s.EnsureProtocolRecorder(c, string(scenario), provider, "actual-X", obs.RecordModeAll)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "proxy-Y")
	AttachRecorderHooks(hc, recorder, "actual-X", provider)

	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_mo"}},
		{Type: "content_block_start", Index: 0, ContentBlock: anthropic.BetaRawContentBlockStartEventContentBlockUnion{Type: "text"}},
		{Type: "content_block_delta", Index: 0, Delta: anthropic.BetaRawMessageStreamEventUnionDelta{Type: "text_delta", Text: "."}},
		{Type: "content_block_stop", Index: 0},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))
	require.NoError(t, s.scenarioRecordSinks[scenario].ForceFlush(ctxWithTimeout(t)))

	records := mem.snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].FinalResponse)
	assert.Equal(t, "actual-X", records[0].FinalResponse.Body["model"],
		"AttachRecorderHooks must override msg.Model with the actualModel arg, not hc.ResponseModel")
}

func TestAttachRecorderHooks_ErrorPath(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	s, mem := newRecordingTestServer(t, scenario, obs.RecordModeAll)
	c, _ := newRecordingTestContext(t, []byte(`{}`))

	provider := &typ.Provider{Name: "p"}
	recorder := s.EnsureProtocolRecorder(c, string(scenario), provider, "m", obs.RecordModeAll)
	require.NotNil(t, recorder)

	hc := protocol.NewHandleContext(c, "rm")
	AttachRecorderHooks(hc, recorder, "m", provider)

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

	require.NoError(t, s.scenarioRecordSinks[scenario].ForceFlush(ctxWithTimeout(t)))

	records := mem.snapshot()
	require.Len(t, records, 1, "exactly one error record must be emitted")
	assert.NotEmpty(t, records[0].Err, "record.Err must be populated on stream error")
	assert.Nil(t, records[0].FinalResponse, "no FinalResponse on error path")
}

func TestAttachRecorderHooks_NilRecorder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := newStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	hc := protocol.NewHandleContext(c, "rm")
	AttachRecorderHooks(hc, nil, "m", &typ.Provider{Name: "p"})

	assert.Empty(t, hc.OnStreamEventHooks)
	assert.Empty(t, hc.OnStreamCompleteHooks)
	assert.Empty(t, hc.OnStreamErrorHooks)
	assert.Empty(t, hc.OnStreamAssembledHooks)

	// And the stream still runs cleanly with no recorder attached.
	events := []*anthropic.BetaRawMessageStreamEventUnion{
		{Type: "message_start", Message: anthropic.BetaMessage{ID: "msg_nil"}},
		{Type: "message_stop"},
	}
	require.NoError(t, driveBetaStream(hc, events))
}

func ctxWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
