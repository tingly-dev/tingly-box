package harnessupstream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// defaultCaptureLimit bounds the capture log so a long-running test upstream
// cannot grow without limit; the harness reads each record right after the
// request that produced it.
const defaultCaptureLimit = 512

// redactedHeaders are credentials the gateway under test sent to this
// instance. They are replaced, not dropped, so a test can still assert that
// the gateway authenticated.
var redactedHeaders = []string{"Authorization", "X-Api-Key", "Api-Key"}

// Capture is one request a /virtual route received.
type Capture struct {
	Seq       int64           `json:"seq"`
	Time      time.Time       `json:"time"`
	Method    string          `json:"method"`
	Path      string          `json:"path"`
	RequestID string          `json:"request_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	Headers   http.Header     `json:"headers"`
	Body      json.RawMessage `json:"body,omitempty"`
	// RawBody carries a body that is not valid JSON, verbatim.
	RawBody string `json:"raw_body,omitempty"`
}

// captureLog is a bounded, concurrency-safe ring of captures, oldest dropped
// first.
type captureLog struct {
	mu      sync.Mutex
	limit   int
	nextSeq int64
	records []Capture
}

func newCaptureLog(limit int) *captureLog {
	return &captureLog{limit: limit}
}

func (l *captureLog) add(r *http.Request, model string, body []byte) {
	headers := r.Header.Clone()
	for _, h := range redactedHeaders {
		if headers.Get(h) != "" {
			headers.Set(h, "[redacted]")
		}
	}
	rec := Capture{
		Time:      time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		RequestID: r.Header.Get(RequestIDHeader),
		Model:     model,
		Headers:   headers,
	}
	if json.Valid(body) {
		rec.Body = append(json.RawMessage(nil), body...)
	} else if len(body) > 0 {
		rec.RawBody = string(body)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextSeq++
	rec.Seq = l.nextSeq
	l.records = append(l.records, rec)
	if over := len(l.records) - l.limit; over > 0 {
		l.records = append(l.records[:0:0], l.records[over:]...)
	}
}

// find returns up to limit records matching the non-empty filters, newest
// first.
func (l *captureLog) find(requestID, model string, limit int) []Capture {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Capture{}
	for i := len(l.records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := l.records[i]
		if requestID != "" && rec.RequestID != requestID {
			continue
		}
		if model != "" && rec.Model != model {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func (l *captureLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = nil
}

func parsePositive(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("not a positive integer: %q", s)
	}
	return n, nil
}
