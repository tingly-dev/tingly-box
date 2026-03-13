package obs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogSource identifies the source of a log entry for routing decisions
type LogSource string

const (
	// LogSourceHTTP indicates logs from HTTP requests
	LogSourceHTTP LogSource = "http"
	// LogSourceSystem indicates logs from the application/system
	LogSourceSystem LogSource = "system"
	// LogSourceAction indicates logs from user actions/operations
	LogSourceAction LogSource = "action"
	// LogSourceUnknown indicates unknown log source
	LogSourceUnknown LogSource = "unknown"
)

// MultiLogger writes logs to multiple outputs: text, JSON, and memory.
// Text logs are for human readability, JSON logs for persistence, and memory for quick API access.
//
// Architecture:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                     MultiLogger                         │
//	│  ┌────────────┐  ┌────────────┐  ┌──────────────────┐  │
//	│  │    Text    │  │    JSON    │  │  Memory Sinks    │  │
//	│  │   Output   │  │   Output   │  │  (by source)     │  │
//	│  └────────────┘  └────────────┘  ├──────────────────┤  │
//	│                                   │ HTTP (1000)      │  │
//	│                                   │ System (500)     │  │
//	│                                   │ Action (100)     │  │
//	│                                   └──────────────────┘  │
//	└─────────────────────────────────────────────────────────┘
//
// Usage:
//
//	// Get the main multi logger
//	logger := obs.NewMultiLogger(config)
//
//	// Get a scoped logger for specific use case
//	actionLogger := logger.WithSource(obs.LogSourceAction)
//	actionLogger.LogAction("add_provider", details, true, "success")
//
//	// Or get the underlying logrus logger for custom use
//	httpLogger := logger.GetLogrusLogger(obs.LogSourceHTTP)
//	httpLogger.WithField("path", "/api/test").Info("request processed")
type MultiLogger struct {
	textWriter io.Writer
	jsonLogger *lumberjack.Logger
	mu         sync.RWMutex
	level      logrus.Level

	// Memory sinks by source - lazy initialization
	memorySinks   map[LogSource]*MemoryLogHook
	memorySinksMu sync.RWMutex

	// logrus loggers by source - cached for performance
	loggers   map[LogSource]*logrus.Logger
	loggersMu sync.RWMutex
}

// LogEntry represents a structured log entry for JSON output
type LogEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Source  string                 `json:"source,omitempty"` // Log source identification
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// MultiLoggerConfig holds configuration for multi-mode logging
type MultiLoggerConfig struct {
	// Text log config (for humans)
	TextLogPath    string
	TextMaxSize    int // MB
	TextMaxBackups int
	TextMaxAge     int // days
	TextCompress   bool

	// JSON log config (for frontend/API)
	JSONLogPath    string
	JSONMaxSize    int // MB (small, for recent logs only)
	JSONMaxBackups int // small number, we only keep recent
	JSONMaxAge     int // days

	// Memory sink config (optional, uses defaults if not specified)
	MemorySinkConfig map[LogSource]MemorySinkConfig
}

// MemorySinkConfig holds configuration for a memory sink
type MemorySinkConfig struct {
	MaxEntries int // Maximum entries to keep in memory (default varies by source)
}

// DefaultMultiLoggerConfig returns default configuration
func DefaultMultiLoggerConfig(configDir string) *MultiLoggerConfig {
	return &MultiLoggerConfig{
		// Text logs - standard retention
		TextLogPath:    filepath.Join(configDir, "log", "tingly-box.log"),
		TextMaxSize:    10,
		TextMaxBackups: 10,
		TextMaxAge:     30,
		TextCompress:   true,

		// JSON logs - short retention for frontend
		JSONLogPath:    filepath.Join(configDir, "log", "tingly-box.jsonl"),
		JSONMaxSize:    5,
		JSONMaxBackups: 3,
		JSONMaxAge:     7,

		// Default memory sink sizes
		MemorySinkConfig: map[LogSource]MemorySinkConfig{
			LogSourceHTTP:   {MaxEntries: 1000}, // HTTP requests: high volume
			LogSourceSystem: {MaxEntries: 500},  // System logs: medium volume
			LogSourceAction: {MaxEntries: 100},  // User actions: low volume, important
		},
	}
}

// NewMultiLogger creates a new multi-mode logger
func NewMultiLogger(cfg *MultiLoggerConfig) (*MultiLogger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.TextLogPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create text logger (lumberjack handles rotation)
	textLogger := &lumberjack.Logger{
		Filename:   cfg.TextLogPath,
		MaxSize:    cfg.TextMaxSize,
		MaxBackups: cfg.TextMaxBackups,
		MaxAge:     cfg.TextMaxAge,
		Compress:   cfg.TextCompress,
	}

	// Create JSON logger (lumberjack handles rotation)
	jsonLogger := &lumberjack.Logger{
		Filename:   cfg.JSONLogPath,
		MaxSize:    cfg.JSONMaxSize,
		MaxBackups: cfg.JSONMaxBackups,
		MaxAge:     cfg.JSONMaxAge,
		Compress:   false, // No compression for fast reading
	}

	ml := &MultiLogger{
		textWriter:  textLogger,
		jsonLogger:  jsonLogger,
		level:       logrus.InfoLevel,
		memorySinks: make(map[LogSource]*MemoryLogHook),
		loggers:     make(map[LogSource]*logrus.Logger),
	}

	// Pre-initialize memory sinks from config
	for source, sinkCfg := range cfg.MemorySinkConfig {
		ml.getOrCreateMemorySink(source, sinkCfg.MaxEntries)
	}

	return ml, nil
}

// WithSource returns a scoped logger for the specified source.
// This provides a convenient API for logging with a specific source.
//
// Example:
//
//	actionLogger := logger.WithSource(LogSourceAction)
//	actionLogger.LogAction("add_provider", details, true, "success")
func (m *MultiLogger) WithSource(source LogSource) *ScopedLogger {
	return &ScopedLogger{
		multiLogger: m,
		source:      source,
	}
}

// GetLogrusLogger returns a logrus.Logger instance that writes to this MultiLogger
// with the specified source. The logger is cached for performance.
//
// Example:
//
//	logger := multiLogger.GetLogrusLogger(LogSourceHTTP)
//	logger.WithField("path", "/api/test").Info("request processed")
func (m *MultiLogger) GetLogrusLogger(source LogSource) *logrus.Logger {
	m.loggersMu.RLock()
	if logger, ok := m.loggers[source]; ok {
		m.loggersMu.RUnlock()
		return logger
	}
	m.loggersMu.RUnlock()

	m.loggersMu.Lock()
	defer m.loggersMu.Unlock()

	// Double-check after acquiring write lock
	if logger, ok := m.loggers[source]; ok {
		return logger
	}

	// Create new logger with hook
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Discard default output
	logger.AddHook(&multiLoggerHook{
		multiLogger: m,
		source:      source,
	})
	logger.SetLevel(m.level)

	m.loggers[source] = logger
	return logger
}

// GetMemorySink returns the memory sink for the specified source, creating it if necessary.
// Returns nil if the source has no memory sink configured.
func (m *MultiLogger) GetMemorySink(source LogSource) *MemoryLogHook {
	m.memorySinksMu.RLock()
	sink, exists := m.memorySinks[source]
	m.memorySinksMu.RUnlock()

	if exists {
		return sink
	}

	// Create with default size based on source
	defaultSize := m.getDefaultMemorySinkSize(source)
	return m.getOrCreateMemorySink(source, defaultSize)
}

// getOrCreateMemorySink creates or returns an existing memory sink
func (m *MultiLogger) getOrCreateMemorySink(source LogSource, maxEntries int) *MemoryLogHook {
	m.memorySinksMu.Lock()
	defer m.memorySinksMu.Unlock()

	// Double-check
	if sink, exists := m.memorySinks[source]; exists {
		return sink
	}

	sink := NewMemoryLogHook(maxEntries)
	m.memorySinks[source] = sink
	return sink
}

// getDefaultMemorySinkSize returns the default memory sink size for a source
func (m *MultiLogger) getDefaultMemorySinkSize(source LogSource) int {
	switch source {
	case LogSourceHTTP:
		return 1000
	case LogSourceSystem:
		return 500
	case LogSourceAction:
		return 100
	default:
		return 100
	}
}

// SetLevel sets the minimum log level for all loggers
func (m *MultiLogger) SetLevel(level logrus.Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.level = level

	// Update all cached loggers
	m.loggersMu.RLock()
	defer m.loggersMu.RUnlock()
	for _, logger := range m.loggers {
		logger.SetLevel(level)
	}
}

// GetLevel returns the current minimum log level
func (m *MultiLogger) GetLevel() logrus.Level {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.level
}

// Write implements io.Writer for text output
func (m *MultiLogger) Write(p []byte) (n int, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.textWriter != nil {
		n, err = m.textWriter.Write(p)
	}

	return n, err
}

// WriteEntry writes a structured log entry to the JSON log and memory sink
func (m *MultiLogger) WriteEntry(entry *logrus.Entry) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if entry.Level > m.level {
		return nil
	}

	// Extract source from fields, default to system if not present
	source := LogSourceSystem
	if src, ok := entry.Data["source"].(string); ok {
		source = LogSource(src)
	}

	// Write to memory sink for this source (if configured)
	m.memorySinksMu.RLock()
	if sink, exists := m.memorySinks[source]; exists {
		// Fire to memory sink (ignore errors, memory is best-effort)
		_ = sink.Fire(entry)
	}
	m.memorySinksMu.RUnlock()

	// Build fields map for JSON
	fields := make(map[string]interface{}, len(entry.Data))
	for k, v := range entry.Data {
		if k != "source" { // Don't duplicate source in fields
			fields[k] = v
		}
	}

	logEntry := LogEntry{
		Time:    entry.Time,
		Level:   entry.Level.String(),
		Message: entry.Message,
		Source:  string(source),
		Fields:  fields,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write to JSON log
	if _, err := m.jsonLogger.Write(append(jsonData, '\n')); err != nil {
		return fmt.Errorf("failed to write to JSON log: %w", err)
	}

	return nil
}

// GetJSONLogPath returns the path to the JSON log file
func (m *MultiLogger) GetJSONLogPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jsonLogger.Filename
}

// ReadJSONLogs reads log entries from the JSON log file with optional filtering
func (m *MultiLogger) ReadJSONLogs(limit int) ([]LogEntry, error) {
	m.mu.RLock()
	logPath := m.jsonLogger.Filename
	m.mu.RUnlock()

	return readLogEntriesBackwards(logPath, limit)
}

// Close closes the logger
func (m *MultiLogger) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	if m.textWriter != nil {
		if closer, ok := m.textWriter.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if m.jsonLogger != nil {
		if err := m.jsonLogger.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing logger: %v", errs)
	}

	return nil
}

// multiLoggerHook is a logrus hook that writes to MultiLogger with a specific source
type multiLoggerHook struct {
	multiLogger *MultiLogger
	source      LogSource
}

func (h *multiLoggerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *multiLoggerHook) Fire(entry *logrus.Entry) error {
	// Inject source into entry
	entry.Data["source"] = string(h.source)
	return h.multiLogger.WriteEntry(entry)
}

// ScopedLogger provides a convenient API for logging with a specific source
type ScopedLogger struct {
	multiLogger *MultiLogger
	source      LogSource
}

// GetLogrusLogger returns the logrus logger for this scope
func (s *ScopedLogger) GetLogrusLogger() *logrus.Logger {
	return s.multiLogger.GetLogrusLogger(s.source)
}

// GetMemorySink returns the memory sink for this scope
func (s *ScopedLogger) GetMemorySink() *MemoryLogHook {
	return s.multiLogger.GetMemorySink(s.source)
}

// GetMemoryEntries returns all log entries from memory for this scope
func (s *ScopedLogger) GetMemoryEntries() []*logrus.Entry {
	sink := s.GetMemorySink()
	if sink == nil {
		return []*logrus.Entry{}
	}
	return sink.GetEntries()
}

// GetMemoryLatest returns the newest N log entries from memory for this scope
func (s *ScopedLogger) GetMemoryLatest(n int) []*logrus.Entry {
	sink := s.GetMemorySink()
	if sink == nil {
		return []*logrus.Entry{}
	}
	return sink.GetLatest(n)
}

// ClearMemory clears all log entries from memory for this scope
func (s *ScopedLogger) ClearMemory() {
	sink := s.GetMemorySink()
	if sink != nil {
		sink.Clear()
	}
}

// readLogEntriesBackwards reads log entries from the end of the file for efficiency
// Returns entries in reverse chronological order (newest first)
func readLogEntriesBackwards(filePath string, limit int) ([]LogEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := stat.Size()

	if fileSize == 0 {
		return []LogEntry{}, nil
	}

	position := fileSize
	bufferSize := int64(4096)

	var entries []LogEntry
	var partialLine []byte

	for position > 0 && (limit <= 0 || len(entries) < limit) {
		readSize := bufferSize
		if position < bufferSize {
			readSize = position
		}
		position -= readSize

		if _, err := file.Seek(position, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek in file: %w", err)
		}

		buffer := make([]byte, readSize)
		if _, err := io.ReadFull(file, buffer); err != nil {
			return nil, fmt.Errorf("failed to read from file: %w", err)
		}

		if len(partialLine) > 0 {
			buffer = append(partialLine, buffer...)
			partialLine = nil
		}

		for i := len(buffer) - 1; i >= 0; i-- {
			char := buffer[i]

			if char != '\n' {
				partialLine = append([]byte{char}, partialLine...)
				continue
			}

			if len(partialLine) > 0 {
				var entry LogEntry
				if err := json.Unmarshal(partialLine, &entry); err == nil {
					// Append entries in order found (newest first since we're reading backwards)
					entries = append(entries, entry)
					if limit > 0 && len(entries) >= limit {
						return entries, nil
					}
				}
				partialLine = nil
			}
		}
	}

	// Handle last line (if file doesn't end with newline)
	if len(partialLine) > 0 {
		var entry LogEntry
		if err := json.Unmarshal(partialLine, &entry); err == nil {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}
