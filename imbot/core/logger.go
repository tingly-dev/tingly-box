package core

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger represents a logger interface
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// LogLevel represents log level
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent
)

// ParseLogLevel parses a log level string
func ParseLogLevel(level string) LogLevel {
	switch level {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "silent":
		return LevelSilent
	default:
		return LevelInfo
	}
}

// defaultLogger is the default logger implementation
type defaultLogger struct {
	level      LogLevel
	timestamps bool
	prefix     string
	logger     *log.Logger
}

// NewLogger creates a new logger from config
func NewLogger(config *LoggingConfig) Logger {
	level := LevelInfo
	timestamps := true
	prefix := "[imbot]"

	if config != nil {
		level = ParseLogLevel(config.Level)
		timestamps = config.Timestamps
	}

	return &defaultLogger{
		level:      level,
		timestamps: timestamps,
		prefix:     prefix,
		logger:     log.New(os.Stdout, "", 0),
	}
}

// BotLogPrefix is the log prefix that says which bot a line belongs to:
// "[imbot telegram/…456789]". Without it, two bots on one platform write
// indistinguishable lines. Generated bot UUIDs are "<unixnano>-imbot", so only
// the tail of the numeric part tells bots apart.
func BotLogPrefix(platform Platform, uuid string) string {
	id := strings.TrimSuffix(uuid, "-imbot")
	if len(id) > 8 {
		id = "…" + id[len(id)-6:]
	}
	if id == "" {
		return "[imbot " + string(platform) + "]"
	}
	return "[imbot " + string(platform) + "/" + id + "]"
}

// NewLoggerWithPrefix creates a new logger with a custom prefix
func NewLoggerWithPrefix(prefix string, level LogLevel, timestamps bool) Logger {
	return &defaultLogger{
		level:      level,
		timestamps: timestamps,
		prefix:     prefix,
		logger:     log.New(os.Stdout, "", 0),
	}
}

// Debug logs a debug message
func (l *defaultLogger) Debug(format string, args ...any) {
	if l.level <= LevelDebug {
		l.log("DEBUG", format, args...)
	}
}

// Info logs an info message
func (l *defaultLogger) Info(format string, args ...any) {
	if l.level <= LevelInfo {
		l.log("INFO", format, args...)
	}
}

// Warn logs a warning message
func (l *defaultLogger) Warn(format string, args ...any) {
	if l.level <= LevelWarn {
		l.log("WARN", format, args...)
	}
}

// Error logs an error message
func (l *defaultLogger) Error(format string, args ...any) {
	if l.level <= LevelError {
		l.log("ERROR", format, args...)
	}
}

// log logs a message with the given level
func (l *defaultLogger) log(level, format string, args ...any) {
	var prefix string

	if l.timestamps {
		prefix = fmt.Sprintf("%s [%s] %s ", time.Now().Format("2006-01-02 15:04:05"), level, l.prefix)
	} else {
		prefix = fmt.Sprintf("[%s] %s ", level, l.prefix)
	}

	message := fmt.Sprintf(prefix+format, args...)
	l.logger.Println(message)
}

// NoopLogger is a logger that doesn't log anything
type NoopLogger struct{}

// NewNoopLogger creates a new no-op logger
func NewNoopLogger() Logger {
	return &NoopLogger{}
}

// Debug does nothing
func (l *NoopLogger) Debug(format string, args ...any) {}

// Info does nothing
func (l *NoopLogger) Info(format string, args ...any) {}

// Warn does nothing
func (l *NoopLogger) Warn(format string, args ...any) {}

// Error does nothing
func (l *NoopLogger) Error(format string, args ...any) {}

// MultiLogger logs to multiple loggers
type MultiLogger struct {
	loggers []Logger
}

// NewMultiLogger creates a new multi-logger
func NewMultiLogger(loggers ...Logger) Logger {
	return &MultiLogger{loggers: loggers}
}

// Debug logs to all loggers
func (l *MultiLogger) Debug(format string, args ...any) {
	for _, logger := range l.loggers {
		logger.Debug(format, args...)
	}
}

// Info logs to all loggers
func (l *MultiLogger) Info(format string, args ...any) {
	for _, logger := range l.loggers {
		logger.Info(format, args...)
	}
}

// Warn logs to all loggers
func (l *MultiLogger) Warn(format string, args ...any) {
	for _, logger := range l.loggers {
		logger.Warn(format, args...)
	}
}

// Error logs to all loggers
func (l *MultiLogger) Error(format string, args ...any) {
	for _, logger := range l.loggers {
		logger.Error(format, args...)
	}
}

// warn logs a warning through the package-wide logrus logger. This is for
// diagnostics emitted by core helpers themselves (e.g. the Segments/Text
// shadow migration warning) — not a replacement for the per-bot Logger
// interface above, which stays injectable for bots. The logrus standard
// logger is the same stream the rest of tingly-box watches.
func warn(format string, args ...any) {
	logrus.Warnf(format, args...)
}

// info logs at info level through the package-wide logrus logger. See warn.
func info(format string, args ...any) {
	logrus.Infof(format, args...)
}
