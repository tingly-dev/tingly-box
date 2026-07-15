package daemon

import (
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogRotationConfig holds configuration for log rotation
type LogRotationConfig struct {
	Filename   string // Log file path
	MaxSize    int    // Maximum size in megabytes
	MaxBackups int    // Maximum number of old log files to retain
	MaxAge     int    // Maximum number of days to retain old log files
	Compress   bool   // Compress old log files
}

// DefaultLogRotationConfig returns default log rotation settings
func DefaultLogRotationConfig(logFile string) *LogRotationConfig {
	return &LogRotationConfig{
		Filename:   logFile,
		MaxSize:    10,   // 10 MB
		MaxBackups: 10,   // Keep 10 old log files
		MaxAge:     30,   // 30 days
		Compress:   true, // Compress rotated files
	}
}

// NewLogger creates a new lumberjack logger with the given configuration
func NewLogger(cfg *LogRotationConfig) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
}

// IsDaemonProcess checks if the current process is running as a daemon
func IsDaemonProcess() bool {
	return os.Getenv("_TINGLY_BOX_DAEMON") == "1"
}

// buildDaemonArgs computes the argv for the re-exec'd daemon child. Daemonize
// re-runs the current command, so a value the parent *resolved* rather than
// received on the command line (e.g. a `restart` preserving the running port)
// would be re-resolved by the child and drift back to the default. overrideArgs
// is appended so the child sees the pinned flag; the CLI parser takes the last
// occurrence, so this wins over any earlier value without needing to strip it.
func buildDaemonArgs(args, overrideArgs []string) []string {
	if len(overrideArgs) == 0 {
		return args
	}
	out := make([]string, 0, len(args)+len(overrideArgs))
	out = append(out, args...)
	out = append(out, overrideArgs...)
	return out
}
