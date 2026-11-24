package protocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

const ProbeErrorCaptureHeader = "X-Tingly-Probe-Error-Capture"

var probeErrorCaptures sync.Map

type ProbeErrorCapture struct {
	mu           sync.Mutex
	err          error
	streamFailed bool
	responseBody string
}

// BeginProbeErrorCapture correlates one in-process probe with its loopback
// gateway request. The token is valid only for the lifetime of this probe.
func BeginProbeErrorCapture() (string, *ProbeErrorCapture, func()) {
	token := uuid.NewString()
	capture := &ProbeErrorCapture{}
	probeErrorCaptures.Store(token, capture)
	return token, capture, func() { probeErrorCaptures.Delete(token) }
}

type probeErrorCaptureKey struct{}

func WithProbeErrorCapture(ctx context.Context, capture *ProbeErrorCapture) context.Context {
	return context.WithValue(ctx, probeErrorCaptureKey{}, capture)
}

func BindProbeErrorCapture(req *http.Request) *http.Request {
	if value, ok := probeErrorCaptures.Load(req.Header.Get(ProbeErrorCaptureHeader)); ok {
		capture := value.(*ProbeErrorCapture)
		capture.mu.Lock()
		capture.err, capture.streamFailed, capture.responseBody = nil, false, ""
		capture.mu.Unlock()
		return req.WithContext(WithProbeErrorCapture(req.Context(), capture))
	}
	return req
}

func HasProbeErrorCapture(ctx context.Context) bool {
	return ctx.Value(probeErrorCaptureKey{}) != nil
}

func CaptureProbeHTTPBody(ctx context.Context, body []byte) {
	if capture, ok := ctx.Value(probeErrorCaptureKey{}).(*ProbeErrorCapture); ok {
		capture.mu.Lock()
		capture.responseBody = string(body)
		capture.mu.Unlock()
	}
}

func CaptureProbeError(req *http.Request, err error, streamFailed bool) {
	if capture, ok := req.Context().Value(probeErrorCaptureKey{}).(*ProbeErrorCapture); ok {
		capture.mu.Lock()
		capture.err = err
		capture.streamFailed = streamFailed
		if streamFailed {
			capture.responseBody = ""
		}
		capture.mu.Unlock()
	}
}

func (c *ProbeErrorCapture) Failure() (err error, streamFailed bool, responseBody string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err, c.streamFailed, c.responseBody
}

// ProbeErrorBody returns the original response body or SSE error data. The
// HTTP SDKs retain a replayable copy of the body on their typed API errors.
func ProbeErrorBody(err error) string {
	var response *http.Response
	var fallback string
	var oai *openai.Error
	var ant *anthropic.Error
	var stream *ssestream.StreamError
	switch {
	case errors.As(err, &oai):
		response, fallback = oai.Response, oai.RawJSON()
	case errors.As(err, &ant):
		response, fallback = ant.Response, ant.RawJSON()
	case errors.As(err, &stream):
		return string(stream.Event.Data)
	}
	if response != nil && response.StatusCode >= http.StatusBadRequest && response.Body != nil {
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if readErr == nil {
			return string(body)
		}
	}
	return fallback
}

type ProbeFailure struct {
	Cause        error
	Upstream     error
	ResponseBody string
}

func (e *ProbeFailure) Error() string { return e.Cause.Error() }
func (e *ProbeFailure) Unwrap() error { return e.Cause }
