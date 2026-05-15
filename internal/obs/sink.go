package obs

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// RecordMode defines which fields are captured by the Sink.
type RecordMode string

const (
	RecordModeAll      RecordMode = "all"      // requests + responses
	RecordModeScenario RecordMode = "scenario" // same as all, scenario-partitioned
	RecordModeSlim     RecordMode = "slim"     // reserved

	RecordModeRequestOnly           RecordMode = "request"                 // Record transformed request only
	RecordModeRequestResponse       RecordMode = "request_response"        // Record transformed request + final response
	RecordModeStagedRequestResponse RecordMode = "staged_request_response" // Record original request + transformed request + final response
)

// RecordRequest represents the HTTP request details. Kept for callers that
// build RecordRequest directly (e.g. client/record_roundtripper.go).
type RecordRequest struct {
	Method  string                 `json:"method"`
	URL     string                 `json:"url"`
	Headers map[string]string      `json:"headers"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

// RecordResponse represents the HTTP response details.
type RecordResponse struct {
	StatusCode   int                    `json:"status_code"`
	Headers      map[string]string      `json:"headers"`
	Body         map[string]interface{} `json:"body,omitempty"`
	IsStreaming  bool                   `json:"is_streaming,omitempty"`
	StreamChunks []string               `json:"-"`
}

// Sink manages recording of LLM request/response cycles.
// All writes are batched asynchronously; Emit is non-blocking.
type Sink struct {
	mode      RecordMode
	baseDir   string
	processor *BatchProcessor
}

// GetMode returns the configured RecordMode, or "" when the sink is nil.
func (s *Sink) GetMode() RecordMode {
	if s == nil {
		return ""
	}
	return s.mode
}

// SinkOption customises Sink construction. Apply via NewSink(baseDir, mode, opts...).
type SinkOption func(*sinkConfig)

type sinkConfig struct {
	// explicitExporters fully replaces the default exporter list when non-nil.
	explicitExporters []RecordExporter
	// enableCAS appends a CASFileExporter alongside the default gzip exporter.
	enableCAS bool
}

// WithCASExporter appends a CASFileExporter to the default gzip exporter.
// Records are written twice (once gzipped, once as content-addressed slim
// JSONL + blobs) — useful for cross-session analysis or replay tooling.
func WithCASExporter() SinkOption {
	return func(c *sinkConfig) { c.enableCAS = true }
}

// WithExporters replaces the Sink's default exporter list entirely. Useful for
// tests and for plugging in future exporters (SQLite, OTLP, remote collectors).
func WithExporters(exporters ...RecordExporter) SinkOption {
	return func(c *sinkConfig) { c.explicitExporters = exporters }
}

// NewSink creates a new Sink backed by the OTel-shaped batch pipeline.
// Returns nil when recording is disabled (empty mode or baseDir).
//
// Default exporter: GzipFileExporter (one gzip member per batch, per-session
// .jsonl.gz files). Pass WithCASExporter() to additionally write
// content-addressed slim JSONL + blobs.
func NewSink(baseDir string, mode RecordMode, opts ...SinkOption) *Sink {
	switch mode {
	case "":
		return nil
	case RecordModeSlim:
		logrus.Warnf("obs: record mode 'slim' is handled automatically, use 'all' or 'scenario'")
		return nil
	case RecordModeAll, RecordModeScenario,
		RecordModeRequestOnly, RecordModeRequestResponse, RecordModeStagedRequestResponse:
		cfg := sinkConfig{}
		for _, opt := range opts {
			opt(&cfg)
		}
		// baseDir is only required when the default file-backed exporters
		// are used. WithExporters supplies a complete exporter list and
		// makes baseDir irrelevant (the test in-memory exporter case).
		if baseDir == "" && len(cfg.explicitExporters) == 0 {
			return nil
		}
		exp, err := buildExporter(baseDir, &cfg)
		if err != nil {
			logrus.Errorf("obs: failed to initialise exporter at %s: %v", baseDir, err)
			return nil
		}
		if exp == nil {
			return nil
		}
		return &Sink{
			mode:      mode,
			baseDir:   baseDir,
			processor: NewBatchProcessor(exp, BatchProcessorOptions{}),
		}
	default:
		logrus.Warnf("obs: unknown record mode %q, recording disabled", mode)
		return nil
	}
}

// buildExporter resolves the configured exporters into a single RecordExporter.
func buildExporter(baseDir string, cfg *sinkConfig) (RecordExporter, error) {
	if len(cfg.explicitExporters) > 0 {
		if len(cfg.explicitExporters) == 1 {
			return cfg.explicitExporters[0], nil
		}
		return NewMultiExporter(cfg.explicitExporters...), nil
	}
	exporters := []RecordExporter{NewGzipFileExporter(baseDir)}
	if cfg.enableCAS {
		cas, err := NewCASFileExporter(baseDir)
		if err != nil {
			return nil, err
		}
		exporters = append(exporters, cas)
	}
	if len(exporters) == 1 {
		return exporters[0], nil
	}
	return NewMultiExporter(exporters...), nil
}

// Emit enqueues r for asynchronous export. The call is non-blocking.
func (s *Sink) Emit(r *Record) {
	if s == nil || s.processor == nil {
		return
	}
	s.processor.Emit(r)
}

// RecordWithScenario builds a single-stage Record (original request + final
// response) and emits it. Used by client-side roundtrippers that don't go
// through the transform pipeline. Server-side code should construct a *Record
// directly and call Emit.
func (s *Sink) RecordWithScenario(provider, model, scenario string, req *RecordRequest, resp *RecordResponse, duration time.Duration, err error) {
	if s == nil {
		return
	}
	r := &Record{
		Timestamp: time.Now().UTC(),
		RequestID: uuid.New().String(),
		Provider:  provider,
		Scenario:  scenario,
		Model:     model,
		Duration:  duration,

		OriginalRequest: req,
		FinalResponse:   resp,
	}
	if err != nil {
		r.Err = err.Error()
	}
	s.Emit(r)
}

// IsEnabled returns whether recording is active.
func (s *Sink) IsEnabled() bool {
	return s != nil && s.processor != nil
}

// GetBaseDir returns the recording root directory.
func (s *Sink) GetBaseDir() string {
	if s == nil {
		return ""
	}
	return s.baseDir
}

// ForceFlush drains any pending records by delegating to the underlying
// processor. Used by tests that need a synchronisation point before
// inspecting exported records.
func (s *Sink) ForceFlush(ctx context.Context) error {
	if s == nil || s.processor == nil {
		return nil
	}
	return s.processor.ForceFlush(ctx)
}

// Close drains pending records and shuts down the pipeline.
func (s *Sink) Close() {
	if s == nil || s.processor == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.processor.Shutdown(ctx); err != nil {
		logrus.Warnf("obs: sink shutdown error: %v", err)
	}
	logrus.Info("obs: recording sink closed")
}

// sanitizeModelForPath replaces invalid filename characters with hyphens.
// Kept for any callers outside the obs package that reference it via sink.
func sanitizeModelForPath(model string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := model
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "-")
	}
	return result
}
