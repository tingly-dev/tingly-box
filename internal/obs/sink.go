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

// RecordEntry is kept for callers in pkg/otel that build it directly.
type RecordEntry struct {
	Timestamp  string                 `json:"timestamp"`
	RequestID  string                 `json:"request_id"`
	Provider   string                 `json:"provider"`
	Scenario   string                 `json:"scenario,omitempty"`
	Model      string                 `json:"model"`
	Request    *RecordRequest         `json:"request"`
	Response   *RecordResponse        `json:"response"`
	DurationMs int64                  `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// RecordEntryV2 is kept for backward compatibility. New code should build
// a *Record and call Sink.Emit instead.
type RecordEntryV2 struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id"`
	Provider  string `json:"provider"`
	Scenario  string `json:"scenario,omitempty"`
	Model     string `json:"model"`

	OriginalRequest    *RecordRequest  `json:"original_request,omitempty"`
	TransformedRequest *RecordRequest  `json:"transformed_request,omitempty"`
	ProviderResponse   *RecordResponse `json:"provider_response,omitempty"`
	FinalResponse      *RecordResponse `json:"final_response,omitempty"`

	DurationMs     int64                  `json:"duration_ms"`
	Error          string                 `json:"error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	TransformSteps []string               `json:"transform_steps,omitempty"`
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

// NewSink creates a new Sink backed by the OTel-shaped batch pipeline.
// Returns nil when recording is disabled (empty mode or baseDir).
func NewSink(baseDir string, mode RecordMode) *Sink {
	switch mode {
	case "":
		return nil
	case RecordModeSlim:
		logrus.Warnf("obs: record mode 'slim' is handled automatically, use 'all' or 'scenario'")
		return nil
	case RecordModeAll, RecordModeScenario,
		RecordModeRequestOnly, RecordModeRequestResponse, RecordModeStagedRequestResponse:
		if baseDir == "" {
			return nil
		}
		exp, err := NewFileExporter(baseDir)
		if err != nil {
			logrus.Errorf("obs: failed to initialise file exporter at %s: %v", baseDir, err)
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

// Emit enqueues r for asynchronous export. The call is non-blocking.
func (s *Sink) Emit(r *Record) {
	if s == nil || s.processor == nil {
		return
	}
	s.processor.Emit(r)
}

// RecordEntryV2 converts a legacy V2 entry to a Record and emits it.
// Deprecated: build a *Record and call Emit directly.
func (s *Sink) RecordEntryV2(entry *RecordEntryV2) {
	if s == nil || entry == nil {
		return
	}
	r := &Record{
		Timestamp: time.Now().UTC(),
		RequestID: entry.RequestID,
		Provider:  entry.Provider,
		Scenario:  entry.Scenario,
		Model:     entry.Model,
		Steps:     entry.TransformSteps,
		Err:       entry.Error,
		Duration:  time.Duration(entry.DurationMs) * time.Millisecond,

		OriginalRequest:    entry.OriginalRequest,
		TransformedRequest: entry.TransformedRequest,
		ProviderResponse:   entry.ProviderResponse,
		FinalResponse:      entry.FinalResponse,
	}
	if r.RequestID == "" {
		r.RequestID = uuid.New().String()
	}
	s.Emit(r)
}

// RecordWithScenario is kept for callers that haven't migrated to Emit.
// Deprecated: build a *Record and call Emit directly.
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

// Record is kept for the oldest callers.
// Deprecated: use Emit.
func (s *Sink) Record(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, err error) {
	s.RecordWithScenario(provider, model, "", req, resp, duration, err)
}

// RecordWithMetadata is kept for older callers.
// Deprecated: use Emit.
func (s *Sink) RecordWithMetadata(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, _ map[string]interface{}, err error) {
	s.RecordWithScenario(provider, model, "", req, resp, duration, err)
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
