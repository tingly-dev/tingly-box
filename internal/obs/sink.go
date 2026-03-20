package obs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// RecordMode defines the recording mode
type RecordMode string

const (
	RecordModeAll      RecordMode = "all" // Record both request and response
	RecordModeScenario RecordMode = "scenario"
	RecordModeSlim     RecordMode = "slim" // TODO: Not implemented yet

	RecordModeRequest           RecordMode = "request"          // Record only requests (original + transformed)
	RecordModeResponse          RecordMode = "response"         // Record only response
	RecordModeV2RequestResponse RecordMode = "request_response" // Record both requests and responses
)

// RecordEntry represents a single recorded request/response pair
type RecordEntry struct {
	Timestamp  string                 `json:"timestamp"`
	RequestID  string                 `json:"request_id"`
	Provider   string                 `json:"provider"`
	Scenario   string                 `json:"scenario,omitempty"` // Scenario: openai, anthropic, claude_code, etc.
	Model      string                 `json:"model"`
	Request    *RecordRequest         `json:"request"`
	Response   *RecordResponse        `json:"response"`
	DurationMs int64                  `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// RecordRequest represents the HTTP request details
type RecordRequest struct {
	Method  string                 `json:"method"`
	URL     string                 `json:"url"`
	Headers map[string]string      `json:"headers"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

// RecordResponse represents the HTTP response details
type RecordResponse struct {
	StatusCode int                    `json:"status_code"`
	Headers    map[string]string      `json:"headers"`
	Body       map[string]interface{} `json:"body,omitempty"`
	// Streaming support
	IsStreaming  bool     `json:"is_streaming,omitempty"`
	StreamChunks []string `json:"-"` // Parsed SSE data chunks
}

// RecordEntryV2 represents a V2 recorded entry with dual-stage request/response recording
// This is used for protocol conversion scenarios where we need to capture:
// - Original request (before transforms)
// - Transformed request (after protocol conversion)
// - Provider response (raw response from provider)
// - Final response (after reverse transformation, if applicable)
type RecordEntryV2 struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id"`
	Provider  string `json:"provider"`
	Scenario  string `json:"scenario,omitempty"` // Scenario: openai, anthropic, claude_code, etc.
	Model     string `json:"model"`

	// Dual-stage request recording
	OriginalRequest    *RecordRequest `json:"original_request,omitempty"`    // Before any transforms
	TransformedRequest *RecordRequest `json:"transformed_request,omitempty"` // After base transform

	// Dual-stage response recording
	ProviderResponse *RecordResponse `json:"provider_response,omitempty"` // Raw response from provider
	FinalResponse    *RecordResponse `json:"final_response,omitempty"`    // Final response to client

	DurationMs     int64                  `json:"duration_ms"`
	Error          string                 `json:"error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	TransformSteps []string               `json:"transform_steps,omitempty"` // Applied transforms
}

// Sink manages recording of HTTP requests/responses to JSONL files
type Sink struct {
	mode    RecordMode
	baseDir string
	fileMap map[string]*recordFile // provider -> file
	mutex   sync.RWMutex
}

// recordFile holds a file handle and its writer
type recordFile struct {
	file        *os.File
	writer      *json.Encoder
	currentHour string // time in YYYY-MM-DD-HH format (hourly rotation)
}

// NewSink creates a new record sink
// mode: empty string = disabled, "all" = record all, "response" = response only
// V2 modes: "request", "response_only", "request_response"
func NewSink(baseDir string, mode RecordMode) *Sink {
	switch mode {
	case "":
		// Empty mode means recording is disabled
		return nil

	case RecordModeSlim:
		// Check for slim mode (not implemented)
		logrus.Warnf("Record mode 'slim' is not implemented yet, please use 'all' or 'response'")
		return nil

	case RecordModeAll, RecordModeScenario,
		RecordModeRequest, RecordModeResponse, RecordModeV2RequestResponse:

		// Ensure base directory exists
		if err := os.MkdirAll(baseDir, 0755); err != nil {
			logrus.Errorf("Failed to create record directory %s: %v", baseDir, err)
			return nil
		}

		return &Sink{
			mode:    mode,
			baseDir: baseDir,
			fileMap: make(map[string]*recordFile),
		}
	default:
		// Invalid mode
		logrus.Warnf("Invalid record mode '%s', recording disabled", mode)
		return nil
	}
}

// Record records a single request/response pair
func (r *Sink) Record(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, err error) {
	if r.mode == "" {
		return
	}

	entry := &RecordEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  uuid.New().String(),
		Provider:   provider,
		Model:      model,
		Response:   resp,
		DurationMs: duration.Milliseconds(),
	}

	// Only include request if mode is "all"
	if r.mode == RecordModeAll {
		entry.Request = req
	}

	if err != nil {
		entry.Error = err.Error()
	}

	r.writeEntry(provider, entry)
}

// RecordWithMetadata records a request/response with additional metadata
func (r *Sink) RecordWithMetadata(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, metadata map[string]interface{}, err error) {
	if r.mode == "" {
		return
	}

	entry := &RecordEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  uuid.New().String(),
		Provider:   provider,
		Model:      model,
		Response:   resp,
		DurationMs: duration.Milliseconds(),
		Metadata:   metadata,
	}

	// Only include request if mode is "all"
	if r.mode == RecordModeAll {
		entry.Request = req
	}

	if err != nil {
		entry.Error = err.Error()
	}

	r.writeEntry(provider, entry)
}

// RecordWithScenario records a request/response with scenario information
// Uses scenario-based file naming (e.g., claude_code-{date}.jsonl)
func (r *Sink) RecordWithScenario(provider, model, scenario string, req *RecordRequest, resp *RecordResponse, duration time.Duration, err error) {
	if r.mode == "" {
		return
	}

	entry := &RecordEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  uuid.New().String(),
		Provider:   provider,
		Scenario:   scenario,
		Model:      model,
		Response:   resp,
		DurationMs: duration.Milliseconds(),
	}

	switch r.mode {
	case RecordModeAll:
		entry.Request = req
	case RecordModeScenario:
		entry.Request = req
	}

	if err != nil {
		entry.Error = err.Error()
	}

	// Use scenario-based file naming
	r.writeEntryWithScenario(scenario, entry)
}

// writeEntry writes an entry to the appropriate file
func (r *Sink) writeEntry(provider string, entry *RecordEntry) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Get current hour for file rotation (YYYY-MM-DD-HH)
	currentHour := time.Now().UTC().Format("2006-01-02-15")

	// Get or create file for this provider
	rf, exists := r.fileMap[provider]
	if !exists || rf.currentHour != currentHour {
		// Close old file if hour changed
		if exists && rf.currentHour != currentHour {
			r.closeFile(rf)
		}

		// Create new file
		filename := filepath.Join(r.baseDir, fmt.Sprintf("%s-%s.jsonl", provider, currentHour))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.Errorf("Failed to open record file %s: %v", filename, err)
			return
		}

		rf = &recordFile{
			file:        file,
			writer:      json.NewEncoder(file),
			currentHour: currentHour,
		}
		r.fileMap[provider] = rf
	}

	// Write entry as JSONL (one JSON object per line)
	if err := rf.writer.Encode(entry); err != nil {
		logrus.Errorf("Failed to write record entry: %v", err)
	}
}

// writeEntryWithScenario writes an entry to a scenario-based file
func (r *Sink) writeEntryWithScenario(scenario string, entry *RecordEntry) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Get current hour for file rotation (YYYY-MM-DD-HH)
	currentHour := time.Now().UTC().Format("2006-01-02-15")

	// Use scenario as the file key
	fileKey := fmt.Sprintf("scenario:%s:%s", scenario, entry.Provider)

	// Get or create file for this scenario
	rf, exists := r.fileMap[fileKey]
	if !exists || rf.currentHour != currentHour {
		// Close old file if hour changed
		if exists && rf.currentHour != currentHour {
			r.closeFile(rf)
		}

		// Create new file with scenario-based naming
		filename := filepath.Join(r.baseDir, fmt.Sprintf("%s.%s.%s.jsonl", scenario, entry.Provider, currentHour))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.Errorf("Failed to open record file %s: %v", filename, err)
			return
		}

		rf = &recordFile{
			file:        file,
			writer:      json.NewEncoder(file),
			currentHour: currentHour,
		}
		r.fileMap[fileKey] = rf
	}

	// Write entry as JSONL (one JSON object per line)
	if err := rf.writer.Encode(entry); err != nil {
		logrus.Errorf("Failed to write record entry: %v", err)
	}
}

// closeFile closes a record file
func (r *Sink) closeFile(rf *recordFile) {
	if rf != nil && rf.file != nil {
		if err := rf.file.Close(); err != nil {
			logrus.Errorf("Failed to close record file: %v", err)
		}
	}
}

// Close closes all open record files
func (r *Sink) Close() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, rf := range r.fileMap {
		r.closeFile(rf)
	}
	r.fileMap = make(map[string]*recordFile)

	if r.mode != "" {
		logrus.Info("Record sink closed")
	}
}

// IsEnabled returns whether recording is enabled
func (r *Sink) IsEnabled() bool {
	return r.mode != ""
}

// GetBaseDir returns the base directory for recordings
func (r *Sink) GetBaseDir() string {
	return r.baseDir
}

// RecordEntryV2 records a V2 entry with dual-stage request/response recording
// This is used for protocol conversion scenarios where we need to capture requests at multiple stages
func (r *Sink) RecordEntryV2(entry *RecordEntryV2) {
	if r == nil || r.mode == "" {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Get current hour for file rotation (YYYY-MM-DD-HH)
	currentHour := time.Now().UTC().Format("2006-01-02-15")

	// Use scenario-based file key if scenario is available, otherwise use provider
	fileKey := fmt.Sprintf("v2:%s:%s", entry.Scenario, entry.Provider)
	if entry.Scenario == "" {
		fileKey = fmt.Sprintf("v2:%s", entry.Provider)
	}

	// Get or create file for this entry
	rf, exists := r.fileMap[fileKey]
	if !exists || rf.currentHour != currentHour {
		// Close old file if hour changed
		if exists && rf.currentHour != currentHour {
			r.closeFile(rf)
		}

		// Create new file with V2 naming
		filename := filepath.Join(r.baseDir, fmt.Sprintf("%s.%s.v2.%s.jsonl", entry.Scenario, entry.Provider, currentHour))
		if entry.Scenario == "" {
			filename = filepath.Join(r.baseDir, fmt.Sprintf("%s.v2.%s.jsonl", entry.Provider, currentHour))
		}

		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
		if err != nil {
			logrus.Errorf("Failed to open V2 record file %s: %v", filename, err)
			return
		}

		rf = &recordFile{
			file:        file,
			writer:      json.NewEncoder(file),
			currentHour: currentHour,
		}
		r.fileMap[fileKey] = rf
	}

	// Write entry as JSONL (one JSON object per line)
	if err := rf.writer.Encode(entry); err != nil {
		logrus.Errorf("Failed to write V2 record entry: %v", err)
	}
}
