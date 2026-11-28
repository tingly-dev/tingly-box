package stream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type captureExporter struct {
	mu   sync.Mutex
	recs []*obs.Record
}

func (e *captureExporter) Export(_ context.Context, rs []*obs.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.recs = append(e.recs, rs...)
	return nil
}

func (e *captureExporter) Shutdown(context.Context) error { return nil }

func (e *captureExporter) snapshot() []*obs.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*obs.Record, len(e.recs))
	copy(out, e.recs)
	return out
}

// TestOpenAIChatStream_PassiveFingerprint_E2E is the end-to-end demonstration:
// a mock upstream serves an OpenAI SSE stream, the real SDK decodes it, the
// production passthrough handler relays it with the recorder tap attached, and
// the emitted record carries a fingerprint built entirely from observed
// traffic (no active probes).
func TestOpenAIChatStream_PassiveFingerprint_E2E(t *testing.T) {
	const (
		chunkRole = `{"id":"chatcmpl-Mock123","object":"chat.completion.chunk","created":1,"model":"mock-model","system_fingerprint":"fp_mock","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`
		chunkText = `{"id":"chatcmpl-Mock123","object":"chat.completion.chunk","created":1,"model":"mock-model","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`
		chunkStop = `{"id":"chatcmpl-Mock123","object":"chat.completion.chunk","created":1,"model":"mock-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
		chunkUse  = `{"id":"chatcmpl-Mock123","object":"chat.completion.chunk","created":1,"model":"mock-model","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15},"system_fingerprint":"fp_mock"}`
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, ev := range []string{chunkRole, chunkText, chunkStop, chunkUse} {
			fmt.Fprintf(w, "data: %s\n\n", ev)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithBaseURL(srv.URL+"/v1"), option.WithAPIKey("test-key"))
	streamResp := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{Model: "mock-model"})

	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	mem := &captureExporter{}
	mode := obs.RecordMode(typ.RecordFinalResponse)
	sink := obs.NewSink("", mode, obs.WithExporters(mem))
	require.NotNil(t, sink)
	defer sink.Close()

	rec, err := recording.NewProtocolRecorder(c, sink, "test", mode, []byte(`{"model":"mock-model"}`))
	require.NoError(t, err)
	rec.EnableStreaming()

	hc := newTestHandleContext(c)
	hc.DisableStreamUsage = false
	hc.WithOnStreamEvent(func(event interface{}) error {
		// Mirrors the tap wiring in protocolserver.streamOpenAIChat.
		rec.RecordStreamChunk("chat.completion.chunk", event)
		return nil
	})

	_, err = HandleOpenAIChatStream(hc, streamResp)
	require.NoError(t, err)
	rec.RecordResponse(&typ.Provider{Name: "mock", APIStyle: "openai"}, "mock-model")

	require.NoError(t, sink.ForceFlush(context.Background()))
	recs := mem.snapshot()
	require.Len(t, recs, 1)

	r := recs[0]
	require.NotNil(t, r.Fingerprint, "the recorder must emit a fingerprint")
	fp := r.Fingerprint
	assert.Equal(t, "through_tb", fp.Channel)
	assert.Equal(t, "chatcmpl-*", fp.IDPattern)
	assert.Equal(t, "fp_mock", fp.SystemFingerprint)
	assert.Equal(t, []string{"stop"}, fp.FinishReasons)
	require.NotNil(t, fp.Usage)
	assert.Equal(t, 12, fp.Usage.InputTokens)
	assert.Equal(t, 3, fp.Usage.OutputTokens)
	assert.Equal(t, 15, fp.Usage.TotalTokens)
	require.NotNil(t, fp.Stream)
	assert.Equal(t, 4, fp.Stream.ChunkCount)
	assert.True(t, fp.Stream.UsageLast, "usage arrived in the final chunk")
	assert.Contains(t, fp.Headers["content-type"], "text/event-stream")
}
