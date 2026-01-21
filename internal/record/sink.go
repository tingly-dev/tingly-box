package record

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
	RecordModeAll      RecordMode = "all"      // Record both request and response
	RecordModeResponse RecordMode = "response" // Record only response
	RecordModeSlim     RecordMode = "slim"     // TODO: Not implemented yet
)

// RecordEntry represents a single recorded request/response pair
type RecordEntry struct {
	Timestamp  string                 `json:"timestamp"`
	RequestID  string                 `json:"request_id"`
	Provider   string                 `json:"provider"`
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
	IsStreaming      bool   `json:"is_streaming,omitempty"`
	StreamedContent  string `json:"streamed_content,omitempty"`
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
func NewSink(baseDir string, mode RecordMode) *Sink {
	// Empty mode means recording is disabled
	if mode == "" {
		return &Sink{
			mode: "",
		}
	}

	// Validate mode
	if mode != RecordModeAll && mode != RecordModeResponse && mode != RecordModeSlim {
		logrus.Warnf("Invalid record mode '%s', recording disabled", mode)
		return &Sink{
			mode: "",
		}
	}

	// Check for slim mode (not implemented)
	if mode == RecordModeSlim {
		logrus.Warnf("Record mode 'slim' is not implemented yet, please use 'all' or 'response'")
		return &Sink{
			mode: "",
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		logrus.Errorf("Failed to create record directory %s: %v", baseDir, err)
		return &Sink{
			mode: "",
		}
	}

	return &Sink{
		mode:    mode,
		baseDir: baseDir,
		fileMap: make(map[string]*recordFile),
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
