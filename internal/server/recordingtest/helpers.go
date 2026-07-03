// Package recordingtest provides shared test helpers for exercising
// internal/server/recording's AttachRecorderHooks/ProtocolRecorder wiring
// through the production *server.ProtocolHandler entry points.
//
// This is a plain (non-_test.go) package rather than an external
// recording_test file because its helpers must be importable both from
// internal/server/recording's own tests and from internal/server's e2e
// tests (anthropic_recording_e2e_test.go) — Go test files, even in an
// external _test package, are never importable from another package.
// Putting the helpers in a normal package that imports internal/server
// avoids that restriction; it stays free of any import cycle because
// internal/server's production code never imports recordingtest.
package recordingtest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"

	"github.com/gin-gonic/gin"
)

// MemExporter captures Records emitted by the BatchProcessor for inspection.
// The mutex is mandatory: BatchProcessor exports on its own goroutine.
type MemExporter struct {
	mu      sync.Mutex
	records []*obs.Record
}

func (m *MemExporter) Export(_ context.Context, rs []*obs.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, rs...)
	return nil
}

func (m *MemExporter) Shutdown(context.Context) error { return nil }

func (m *MemExporter) Snapshot() []*obs.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*obs.Record, len(m.records))
	copy(out, m.records)
	return out
}

// StreamableRecorder adds CloseNotifier to httptest.ResponseRecorder so
// gin.Context.Stream (which ProcessStream calls) doesn't panic.
type StreamableRecorder struct {
	*httptest.ResponseRecorder
	closeCh chan bool
}

func NewStreamableRecorder() *StreamableRecorder {
	return &StreamableRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeCh:          make(chan bool, 1),
	}
}

func (s *StreamableRecorder) CloseNotify() <-chan bool { return s.closeCh }

// NewRecordingTestHandler builds a *server.ProtocolHandler whose
// GetOrCreateScenarioSink callback always returns the same in-memory-backed
// sink for scenario, mirroring what root's *Server does via its
// scenarioRecordSinks map.
func NewRecordingTestHandler(t *testing.T, scenario typ.RuleScenario, mode obs.RecordMode) (*server.ProtocolHandler, *obs.Sink, *MemExporter) {
	t.Helper()
	mem := &MemExporter{}
	sink := obs.NewSink("", mode, obs.WithExporters(mem))
	require.NotNil(t, sink, "obs.NewSink must succeed with WithExporters")

	h := server.NewHandler(server.ProtocolHandlerDeps{
		GetOrCreateScenarioSink: func(s typ.RuleScenario) *obs.Sink {
			if s == scenario {
				return sink
			}
			return nil
		},
	})
	t.Cleanup(func() { sink.Close() })
	return h, sink, mem
}

// NewRecordingTestContext builds a gin.Context wired to a StreamableRecorder,
// with a POST /v1/messages request carrying body as its JSON payload.
func NewRecordingTestContext(t *testing.T, body []byte) (*gin.Context, *StreamableRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := NewStreamableRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func CtxWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
