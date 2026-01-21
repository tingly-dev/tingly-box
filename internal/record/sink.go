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
	enabled bool
	baseDir string
	fileMap map[string]*recordFile // provider -> file
	mutex   sync.RWMutex
}

// recordFile holds a file handle and its writer
type recordFile struct {
	file        *os.File
	writer      *json.Encoder
	currentDate string // date in YYYY-MM-DD format
}

// NewSink creates a new record sink
func NewSink(baseDir string, enabled bool) *Sink {
	if !enabled {
		return &Sink{
			enabled: false,
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		logrus.Errorf("Failed to create record directory %s: %v", baseDir, err)
		return &Sink{
			enabled: false,
		}
	}

	return &Sink{
		enabled: true,
		baseDir: baseDir,
		fileMap: make(map[string]*recordFile),
	}
}

// Record records a single request/response pair
func (r *Sink) Record(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, err error) {
	if !r.enabled {
		return
	}

	entry := &RecordEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  uuid.New().String(),
		Provider:   provider,
		Model:      model,
		Request:    req,
		Response:   resp,
		DurationMs: duration.Milliseconds(),
	}

	if err != nil {
		entry.Error = err.Error()
	}

	r.writeEntry(provider, entry)
}

// RecordWithMetadata records a request/response with additional metadata
func (r *Sink) RecordWithMetadata(provider, model string, req *RecordRequest, resp *RecordResponse, duration time.Duration, metadata map[string]interface{}, err error) {
	if !r.enabled {
		return
	}

	entry := &RecordEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		RequestID:  uuid.New().String(),
		Provider:   provider,
		Model:      model,
		Request:    req,
		Response:   resp,
		DurationMs: duration.Milliseconds(),
		Metadata:   metadata,
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

	// Get current date for file rotation
	currentDate := time.Now().UTC().Format("2006-01-02")

	// Get or create file for this provider
	rf, exists := r.fileMap[provider]
	if !exists || rf.currentDate != currentDate {
		// Close old file if date changed
		if exists && rf.currentDate != currentDate {
			r.closeFile(rf)
		}

		// Create new file
		filename := filepath.Join(r.baseDir, fmt.Sprintf("%s-%s.jsonl", provider, currentDate))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.Errorf("Failed to open record file %s: %v", filename, err)
			return
		}

		rf = &recordFile{
			file:        file,
			writer:      json.NewEncoder(file),
			currentDate: currentDate,
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

	if r.enabled {
		logrus.Info("Record sink closed")
	}
}

// IsEnabled returns whether recording is enabled
func (r *Sink) IsEnabled() bool {
	return r.enabled
}

// GetBaseDir returns the base directory for recordings
func (r *Sink) GetBaseDir() string {
	return r.baseDir
}
