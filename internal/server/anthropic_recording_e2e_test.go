package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---- Tier 3: light E2E for the plain Anthropic streaming path ----
//
// Goal: confirm that handleAnthropicStreamResponseV1Beta still calls
// AttachRecorderHooks (and therefore that a streamed response gets
// recorded via the new HandleContext.streamAssembler path).
//
// This test calls handleAnthropicStreamResponseV1Beta directly with a
// synthetic ssestream.Stream rather than spinning up the full gin router
// + httptest upstream + routing pipeline. The wiring assertion is the
// same and the test is ~150 lines lighter.

// fakeDecoder implements ssestream.Decoder over a static event slice.
type fakeDecoder struct {
	events []ssestream.Event
	i      int
}

func (f *fakeDecoder) Next() bool {
	if f.i >= len(f.events) {
		return false
	}
	f.i++
	return true
}

func (f *fakeDecoder) Event() ssestream.Event { return f.events[f.i-1] }
func (f *fakeDecoder) Close() error           { return nil }
func (f *fakeDecoder) Err() error             { return nil }

func TestAnthropicV1BetaStream_Recorded(t *testing.T) {
	const scenario = typ.RuleScenario("test")

	// Wire an in-memory exporter so we can inspect what was recorded.
	mem := &memExporter{}
	sink := obs.NewSink("", obs.RecordModeStagedRequestResponse, obs.WithExporters(mem))
	require.NotNil(t, sink)
	t.Cleanup(func() { sink.Close() })

	s := &Server{
		scenarioRecordSinks: map[typ.RuleScenario]*obs.Sink{scenario: sink},
		recordMode:          obs.RecordModeStagedRequestResponse,
	}

	// Gin context with CloseNotify-capable writer (gin.Context.Stream needs it).
	gin.SetMode(gin.TestMode)
	w := newStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)

	provider := &typ.Provider{Name: "anthropic-test-prov"}

	// Recorder built via the production entry point.
	recorder := s.EnsureProtocolRecorder(c, string(scenario), provider, "actual-stream-model", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, recorder)

	// Synthetic SSE event sequence. The Stream decoder will JSON-unmarshal
	// each event's Data into a *BetaRawMessageStreamEventUnion.
	events := []ssestream.Event{
		{Type: "message_start", Data: []byte(`{"type":"message_start","message":{"id":"msg_e2e","type":"message","role":"assistant","model":"actual-stream-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":0}}}`)},
		{Type: "content_block_start", Data: []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)},
		{Type: "content_block_delta", Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"streamed "}}`)},
		{Type: "content_block_delta", Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"reply"}}`)},
		{Type: "content_block_stop", Data: []byte(`{"type":"content_block_stop","index":0}`)},
		{Type: "message_delta", Data: []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":3,"output_tokens":17}}`)},
		{Type: "message_stop", Data: []byte(`{"type":"message_stop"}`)},
	}
	streamResp := ssestream.NewStream[anthropic.BetaRawMessageStreamEventUnion](&fakeDecoder{events: events}, nil)

	req := &anthropic.BetaMessageNewParams{Model: anthropic.Model("actual-stream-model")}

	// Direct call into the inner streaming handler — the function under
	// test. If AttachRecorderHooks were dropped, the assertion below
	// (assembled body in FinalResponse) would fail.
	s.streamAnthropicBeta(c, req, streamResp, string(req.Model), "proxy-stream-model", provider, recorder)

	require.NoError(t, sink.ForceFlush(ctxWithTimeout(t)))

	records := mem.snapshot()
	require.Len(t, records, 1, "exactly one recording must be emitted by the streaming handler")

	rec := records[0]
	assert.Equal(t, string(scenario), rec.Scenario)
	require.NotNil(t, rec.FinalResponse, "FinalResponse must be populated by AttachRecorderHooks")
	body := rec.FinalResponse.Body
	require.NotNil(t, body, "FinalResponse.Body must contain the assembled message")

	assert.Equal(t, "msg_e2e", body["id"], "assembled message id must reach the recording")
	assert.Equal(t, "actual-stream-model", body["model"],
		"AttachRecorderHooks must override the recorded body's model to actualModel")
	assert.Equal(t, "end_turn", body["stop_reason"])

	content, ok := body["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1)
	block, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "streamed reply", block["text"], "stream deltas must be assembled by HandleContext.streamAssembler")

	usage, ok := body["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 17, usage["output_tokens"], "usage from message_delta must propagate via assembler internal tracking")
}
