package llmclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"tingly-box/internal/record"
)

// RecordRoundTripper is an http.RoundTripper that records requests and responses
type RecordRoundTripper struct {
	transport  http.RoundTripper
	recordSink *record.Sink
	provider   string
	model      string
}

// NewRecordRoundTripper creates a new record round tripper
func NewRecordRoundTripper(transport http.RoundTripper, recordSink *record.Sink, provider, model string) *RecordRoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &RecordRoundTripper{
		transport:  transport,
		recordSink: recordSink,
		provider:   provider,
		model:      model,
	}
}

// RoundTrip executes a single HTTP transaction and records request/response
func (r *RecordRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()

	// Prepare request record
	reqRecord := &record.RecordRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headerToMap(req.Header),
	}

	// Read request body if present
	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
			req.Body.Close()
			// Try to parse as JSON
			var jsonObj interface{}
			if json.Unmarshal(bodyBytes, &jsonObj) == nil {
				if objMap, ok := jsonObj.(map[string]interface{}); ok {
					reqRecord.Body = objMap
				}
			}
			// Restore the body for the actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	// Execute the request
	resp, err := r.transport.RoundTrip(req)
	duration := time.Since(startTime)

	var respRecord *record.RecordResponse
	if resp != nil {
		respRecord = &record.RecordResponse{
			StatusCode: resp.StatusCode,
			Headers:    headerToMap(resp.Header),
		}

		// Check if this is a streaming response
		isStreaming := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

		// Handle response body
		if resp.Body != nil && resp.Body != http.NoBody {
			if isStreaming {
				// For streaming responses, use recordingReader to capture data as it's read
				respRecord.IsStreaming = true

				// Wrap the body with recordingReader
				resp.Body = newRecordingReader(resp.Body, func(content string) {
					// This callback is called when the stream is closed
					// Try to parse as JSON for the Body field
					var jsonObj any
					if json.Unmarshal([]byte(content), &jsonObj) == nil {
						if objMap, ok := jsonObj.(map[string]any); ok {
							respRecord.Body = objMap
						}
					}
					// Store raw streamed content
					respRecord.StreamedContent = content
				})
			} else {
				// For non-streaming responses, read the entire body
				bodyBytes, readErr := io.ReadAll(resp.Body)
				if readErr == nil && len(bodyBytes) > 0 {
					resp.Body.Close()
					// Try to parse as JSON
					var jsonObj any
					if json.Unmarshal(bodyBytes, &jsonObj) == nil {
						if objMap, ok := jsonObj.(map[string]any); ok {
							respRecord.Body = objMap
						}
					}
					// Restore the body for the actual response
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
		}
	}

	// Record the request/response
	if r.recordSink != nil && r.recordSink.IsEnabled() {
		r.recordSink.Record(r.provider, r.model, reqRecord, respRecord, duration, err)
	}

	return resp, err
}

// recordingReader wraps an io.ReadCloser and records all data read from it
type recordingReader struct {
	source    io.ReadCloser
	buffer    *bytes.Buffer
	onClose   func(content string)
	closeOnce sync.Once
	closed    bool
}

func newRecordingReader(source io.ReadCloser, onClose func(string)) *recordingReader {
	return &recordingReader{
		source:  source,
		buffer:  &bytes.Buffer{},
		onClose: onClose,
	}
}

func (r *recordingReader) Read(p []byte) (n int, err error) {
	n, err = r.source.Read(p)
	if n > 0 {
		r.buffer.Write(p[:n])
	}
	return n, err
}

func (r *recordingReader) Close() error {
	err := r.source.Close()
	r.closeOnce.Do(func() {
		r.closed = true
		if r.onClose != nil {
			r.onClose(r.buffer.String())
		}
	})
	return err
}

// headerToMap converts http.Header to map[string]string
func headerToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
