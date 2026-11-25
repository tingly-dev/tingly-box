package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type recordTapRT func(*http.Request) (*http.Response, error)

func (f recordTapRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// countingBody counts Read calls so a test can prove the transport never
// consumes the response body.
type countingBody struct {
	r     io.Reader
	reads int32
}

func (b *countingBody) Read(p []byte) (int, error) {
	atomic.AddInt32(&b.reads, 1)
	return b.r.Read(p)
}

func (b *countingBody) Close() error { return nil }

// recordingExporter captures emitted records for assertions.
type recordingExporter struct {
	mu      sync.Mutex
	records []*obs.Record
}

func (e *recordingExporter) Export(_ context.Context, rs []*obs.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, rs...)
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error { return nil }

func (e *recordingExporter) snapshot() []*obs.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*obs.Record(nil), e.records...)
}

// newUpstreamTestRecorder builds a real ProtocolRecorder backed by an in-memory
// exporter, as the handler would, and returns the request context carrying it.
func newUpstreamTestRecorder(t *testing.T, mode obs.RecordMode) (context.Context, *recording.ProtocolRecorder, *obs.Sink, *recordingExporter) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	exp := &recordingExporter{}
	sink := obs.NewSink("", mode, obs.WithExporters(exp))
	require.NotNil(t, sink)
	t.Cleanup(func() { sink.Close() })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))

	rec, err := recording.NewProtocolRecorder(c, sink, "test", mode, []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, rec)
	rec.BindProvider(&typ.Provider{Name: "p", APIStyle: "openai"}, "m", mode)

	return recording.WithRecorder(context.Background(), rec), rec, sink, exp
}

func TestRecordTransport_RecordsStatusAndAllowlistedHeaders(t *testing.T) {
	ctx, rec, sink, exp := newUpstreamTestRecorder(t, obs.RecordMode(typ.RecordFinalResponse))

	body := &countingBody{r: strings.NewReader(`{"ok":true}`)}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type":                       {"application/json"},
			"X-Request-Id":                       {"req_0123456789abcdef"},
			"Anthropic-Ratelimit-Requests-Limit": {"1000"},
			"Set-Cookie":                         {"sid=secret"},
			"Authorization":                      {"Bearer secret"},
			"Openai-Organization":                {"org-account-id"},
			"X-Some-Internal-Debug-Header":       {"nope"},
		},
		Body: body,
	}

	seenCtx := false
	inner := recordTapRT(func(r *http.Request) (*http.Response, error) {
		// The recorder must be reachable through the *http.Request context.
		if recording.RecorderFromContext(r.Context()) == rec {
			seenCtx = true
		}
		return resp, nil
	})

	got, err := wrapWithRecordTransport(inner).RoundTrip(
		httptest.NewRequest(http.MethodPost, "http://upstream.local/v1/chat/completions", nil).WithContext(ctx))
	require.NoError(t, err)
	assert.True(t, seenCtx, "recorder must ride the request context to the wire transport")
	require.Same(t, resp, got, "the response object is returned unchanged")
	assert.Equal(t, int32(0), atomic.LoadInt32(&body.reads), "the tap must not read the body")

	// The body is still fully available to the caller afterwards.
	data, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(data))

	rec.RecordResponse(&typ.Provider{Name: "p", APIStyle: "openai"}, "m")
	require.NoError(t, sink.ForceFlush(context.Background()))

	records := exp.snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Fingerprint)
	up := records[0].Fingerprint.UpstreamResponse
	require.NotNil(t, up, "upstream_response must be populated from the wire tap")
	assert.Equal(t, http.StatusTooManyRequests, up.StatusCode)
	assert.Equal(t, "application/json", up.Headers["content-type"])
	assert.Equal(t, "req_*", up.Headers["x-request-id"], "id-bearing values are patternized")
	assert.Equal(t, "1000", up.Headers["anthropic-ratelimit-requests-limit"], "vendor-chain headers are kept")
	for _, dropped := range []string{"set-cookie", "authorization", "openai-organization", "x-some-internal-debug-header"} {
		_, ok := up.Headers[dropped]
		assert.False(t, ok, "%s must not be persisted", dropped)
	}
}

func TestRecordTransport_NoRecorderIsPassThrough(t *testing.T) {
	body := &countingBody{r: strings.NewReader("hello")}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Server": {"x"}}, Body: body}
	inner := recordTapRT(func(*http.Request) (*http.Response, error) { return resp, nil })

	// No recorder in context: recording is off.
	got, err := wrapWithRecordTransport(inner).RoundTrip(
		httptest.NewRequest(http.MethodGet, "http://upstream.local/", nil))
	require.NoError(t, err)
	require.Same(t, resp, got)
	assert.Equal(t, int32(0), atomic.LoadInt32(&body.reads))
	data, _ := io.ReadAll(got.Body)
	assert.Equal(t, "hello", string(data))
}

func TestRecordTransport_StreamingChunksUntouched(t *testing.T) {
	// Simulate an SSE stream: the tap must not consume a single chunk before
	// the caller reads the body, and reading afterwards must yield every chunk
	// intact and in order.
	chunks := []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n",
		"data: [DONE]\n\n",
	}
	body := &countingBody{r: &chunkReader{chunks: chunks}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"req_stream"}},
		Body:       body,
	}

	ctx, rec, sink, exp := newUpstreamTestRecorder(t, obs.RecordMode(typ.RecordFinalResponse))
	inner := recordTapRT(func(*http.Request) (*http.Response, error) { return resp, nil })

	got, err := wrapWithRecordTransport(inner).RoundTrip(
		httptest.NewRequest(http.MethodPost, "http://upstream.local/v1/chat/completions", nil).WithContext(ctx))
	require.NoError(t, err)
	require.Same(t, resp, got)
	assert.Equal(t, int32(0), atomic.LoadInt32(&body.reads), "header tap must not touch a streaming body")

	var gotChunks []string
	for {
		line, err := readUntilDoubleNewline(got.Body)
		if line != "" {
			gotChunks = append(gotChunks, line)
		}
		if err != nil {
			break
		}
	}
	assert.Equal(t, chunks, gotChunks, "every chunk survives unchanged and in order")

	rec.EnableStreaming()
	rec.RecordResponse(&typ.Provider{Name: "p", APIStyle: "openai"}, "m")
	require.NoError(t, sink.ForceFlush(context.Background()))
	records := exp.snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Fingerprint.UpstreamResponse)
	assert.Equal(t, "req_*", records[0].Fingerprint.UpstreamResponse.Headers["x-request-id"])
}

// chunkReader delivers the chunks in order, honouring partial reads.
type chunkReader struct {
	chunks []string
	i      int
	off    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	for r.i < len(r.chunks) {
		remaining := r.chunks[r.i][r.off:]
		if len(remaining) == 0 {
			r.i++
			r.off = 0
			continue
		}
		n := copy(p, remaining)
		r.off += n
		if r.off >= len(r.chunks[r.i]) {
			r.i++
			r.off = 0
		}
		return n, nil
	}
	return 0, io.EOF
}

// readUntilDoubleNewline reads one SSE event (through the blank line).
func readUntilDoubleNewline(r io.Reader) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b.WriteByte(buf[0])
			if strings.HasSuffix(b.String(), "\n\n") {
				return b.String(), nil
			}
		}
		if err != nil {
			return b.String(), err
		}
	}
}
