package daemon

import (
	"os"
	"strings"

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
// re-runs the current command, so any value the parent *resolved* (rather than
// received on the command line) would be re-resolved by the child and could
// drift — e.g. a `restart` that preserves the running port, or a port that came
// from config. overrideArgs pins those resolved flags: the matching flags are
// stripped from the original args and replaced, so the child binds exactly what
// the parent decided. With no overrides the original args pass through unchanged.
func buildDaemonArgs(args, overrideArgs []string) []string {
	if len(overrideArgs) == 0 {
		return args
	}
	// Only --port is overridden today; strip both spellings and short form.
	filtered := stripFlag(args, "--port", "-p")
	return append(filtered, overrideArgs...)
}

// stripFlag removes the named flags and their values from args, handling both
// "--flag value" and "--flag=value" (and the same for short forms).
func stripFlag(args []string, names ...string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		drop := false
		for _, name := range names {
			if arg == name {
				drop = true
				i++ // also skip the following value token
				break
			}
			if strings.HasPrefix(arg, name+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, arg)
		}
	}
	return out
}
