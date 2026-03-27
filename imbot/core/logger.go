package core

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Logger represents a logger interface
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
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
func (l *defaultLogger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.log("DEBUG", format, args...)
	}
}

// Info logs an info message
func (l *defaultLogger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.log("INFO", format, args...)
	}
}

// Warn logs a warning message
func (l *defaultLogger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.log("WARN", format, args...)
	}
}

// Error logs an error message
func (l *defaultLogger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.log("ERROR", format, args...)
	}
}

// log logs a message with the given level
func (l *defaultLogger) log(level, format string, args ...interface{}) {
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
func (l *NoopLogger) Debug(format string, args ...interface{}) {}

// Info does nothing
func (l *NoopLogger) Info(format string, args ...interface{}) {}

// Warn does nothing
func (l *NoopLogger) Warn(format string, args ...interface{}) {}

// Error does nothing
func (l *NoopLogger) Error(format string, args ...interface{}) {}

// MultiLogger logs to multiple loggers
type MultiLogger struct {
	loggers []Logger
}

// NewMultiLogger creates a new multi-logger
func NewMultiLogger(loggers ...Logger) Logger {
	return &MultiLogger{loggers: loggers}
}

// Debug logs to all loggers
func (l *MultiLogger) Debug(format string, args ...interface{}) {
	for _, logger := range l.loggers {
		logger.Debug(format, args...)
	}
}

// Info logs to all loggers
func (l *MultiLogger) Info(format string, args ...interface{}) {
	for _, logger := range l.loggers {
		logger.Info(format, args...)
	}
}

// Warn logs to all loggers
func (l *MultiLogger) Warn(format string, args ...interface{}) {
	for _, logger := range l.loggers {
		logger.Warn(format, args...)
	}
}

// Error logs to all loggers
func (l *MultiLogger) Error(format string, args ...interface{}) {
	for _, logger := range l.loggers {
		logger.Error(format, args...)
	}
}
