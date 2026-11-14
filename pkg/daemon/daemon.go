package daemon

import (
	"fmt"
	"os"
	"os/exec"

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

// Daemonize detaches the process from the terminal and runs it in the
// background by re-executing the current command as a detached child
// (session leader on Unix, detached process on Windows) and exiting the
// parent.
//
// Callers that want to know whether the child actually came up should use
// Spawn instead and keep the parent alive until it has.
//
// overrideArgs pins flags the parent resolved (e.g. "--port", "9000") so the
// detached child binds exactly what the parent decided instead of re-resolving
// its own command line. See buildDaemonArgs.
func Daemonize(overrideArgs ...string) error {
	// Check if we're already the child process
	if IsDaemonProcess() {
		return nil
	}
	if _, err := Spawn(overrideArgs...); err != nil {
		return err
	}

	// Parent process exits
	os.Exit(0)
	return nil
}

// Spawn re-executes the current command as a detached daemon child and
// returns without exiting the parent. The returned channel yields the child's
// exit status (nil for a clean exit) once it ends, so the parent can tell a
// child that came up from one that died on startup. The child outlives the
// parent: it is its own session leader and its stdio is detached.
func Spawn(overrideArgs ...string) (<-chan error, error) {
	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get original arguments, pinning any resolved flags for the child
	args := buildDaemonArgs(os.Args[1:], overrideArgs)

	// Set environment variable to mark the child as daemon
	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), "_TINGLY_BOX_DAEMON=1")

	// Redirect stdin, stdout, stderr
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Detach from the terminal (platform-specific attributes)
	cmd.SysProcAttr = daemonSysProcAttr()

	// Start the daemonized process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start daemon process: %w", err)
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
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
