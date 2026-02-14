package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Level represents audit log level
type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// String returns the string representation of the level
func (l Level) String() string {
	return string(l)
}

// Entry represents an audit log entry
type Entry struct {
	Timestamp  time.Time              `json:"timestamp"`
	Level      Level                  `json:"level"`
	Action     string                 `json:"action"`
	UserID     string                 `json:"user_id,omitempty"`
	ClientIP   string                 `json:"client_ip,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	Success    bool                   `json:"success"`
	Message    string                 `json:"message,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
}

// Logger handles audit logging
type Logger struct {
	mu         sync.RWMutex
	entries    []Entry
	maxEntries int
	console    bool
}

// Config holds logger configuration
type Config struct {
	Console    bool // Log to console in addition to file
	MaxEntries int  // Maximum entries to keep in memory (0 = unlimited)
}

// NewLogger creates a new audit logger
func NewLogger(cfg Config) *Logger {
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 10000
	}

	return &Logger{
		entries:    make([]Entry, 0, cfg.MaxEntries),
		maxEntries: cfg.MaxEntries,
		console:    cfg.Console,
	}
}

// Log records an audit entry
func (l *Logger) Log(entry Entry) {
	entry.Timestamp = time.Now().UTC()

	// Add to memory
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxEntries {
		l.entries = l.entries[len(l.entries)-l.maxEntries:]
	}
	l.mu.Unlock()

	// Log to console with structured format
	l.logToConsole(entry)
}

// Info logs an info level entry
func (l *Logger) Info(action, userID, clientIP, message string, details map[string]interface{}) {
	l.Log(Entry{
		Level:    LevelInfo,
		Action:   action,
		UserID:   userID,
		ClientIP: clientIP,
		Success:  true,
		Message:  message,
		Details:  details,
	})
}

// Warn logs a warning level entry
func (l *Logger) Warn(action, userID, clientIP, message string, details map[string]interface{}) {
	l.Log(Entry{
		Level:    LevelWarn,
		Action:   action,
		UserID:   userID,
		ClientIP: clientIP,
		Success:  true,
		Message:  message,
		Details:  details,
	})
}

// Error logs an error level entry
func (l *Logger) Error(action, userID, clientIP, message string, success bool, details map[string]interface{}) {
	l.Log(Entry{
		Level:    LevelError,
		Action:   action,
		UserID:   userID,
		ClientIP: clientIP,
		Success:  success,
		Message:  message,
		Details:  details,
	})
}

// LogRequest logs a complete API request
func (l *Logger) LogRequest(action, userID, clientIP, sessionID, requestID string, success bool, duration time.Duration, details map[string]interface{}) {
	l.Log(Entry{
		Action:     action,
		UserID:     userID,
		ClientIP:   clientIP,
		SessionID:  sessionID,
		RequestID:  requestID,
		Success:    success,
		DurationMs: duration.Milliseconds(),
		Details:    details,
	})
}

// GetEntries returns all audit entries
func (l *Logger) GetEntries() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]Entry, len(l.entries))
	copy(result, l.entries)
	return result
}

// GetLogs returns all audit entries as pointer slice (for API responses)
func (l *Logger) GetLogs() []*Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*Entry, len(l.entries))
	for i := range l.entries {
		result[i] = &l.entries[i]
	}
	return result
}

// GetEntriesByUser returns entries for a specific user
func (l *Logger) GetEntriesByUser(userID string) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []Entry
	for _, entry := range l.entries {
		if entry.UserID == userID {
			result = append(result, entry)
		}
	}
	return result
}

// GetEntriesBySession returns entries for a specific session
func (l *Logger) GetEntriesBySession(sessionID string) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []Entry
	for _, entry := range l.entries {
		if entry.SessionID == sessionID {
			result = append(result, entry)
		}
	}
	return result
}

// ExportJSON exports all entries as JSON
func (l *Logger) ExportJSON() ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return json.MarshalIndent(l.entries, "", "  ")
}

// ExportJSONToFile exports entries to a file
func (l *Logger) ExportJSONToFile(path string) error {
	data, err := l.ExportJSON()
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Clear removes all entries
func (l *Logger) Clear() {
	l.mu.Lock()
	l.entries = l.entries[:0]
	l.mu.Unlock()
}

// Size returns the number of entries
func (l *Logger) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// logToConsole logs to stdout in structured format
func (l *Logger) logToConsole(entry Entry) {
	// Use logrus for structured logging
	logger := logrus.WithFields(logrus.Fields{
		"audit":   true,
		"action":  entry.Action,
		"user":    entry.UserID,
		"ip":      entry.ClientIP,
		"session": entry.SessionID,
		"success": entry.Success,
	})

	switch entry.Level {
	case LevelWarn:
		logger.Warn(entry.Message)
	case LevelError:
		logger.Error(entry.Message)
	default:
		logger.Info(entry.Message)
	}
}
