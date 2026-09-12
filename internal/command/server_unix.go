//go:build !windows

package command

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/internal/lock"
)

// stopProcessGracefully attempts to stop a process gracefully on Unix
// First tries SIGTERM, then falls back to SIGKILL if needed
func stopProcessGracefully(process *os.Process) error {
	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}
	return nil
}

// stopProcessForce forcefully terminates a process using SIGKILL
func stopProcessForce(process *os.Process) error {
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}
	return nil
}

// stopServerWithFileLock stops the running server using the file lock (Unix version)
func stopServerWithFileLock(fileLock *lock.FileLock) error {
	// Get PID from lock file
	pid, err := fileLock.GetPID()
	if err != nil {
		return fmt.Errorf("lock file does not exist or is invalid: %w", err)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := stopProcessGracefully(process); err != nil {
		return fmt.Errorf("failed to send shutdown signal: %w", err)
	}

	// Wait for process to exit. Now that the lock is only released after the
	// server has actually stopped (including closing its HTTP listener and
	// disconnecting MCP sources), graceful shutdown can legitimately take a
	// few seconds — give it more room than a single MCP round-trip before
	// falling back to a force kill. Poll finer than the ceiling so a shutdown
	// that finishes early is observed promptly instead of on the next
	// whole-second tick.
	const pollInterval = 150 * time.Millisecond
	const waitCeiling = 10 * time.Second
	for deadline := time.Now().Add(waitCeiling); time.Now().Before(deadline); time.Sleep(pollInterval) {
		if !fileLock.IsLocked() {
			_ = fileLock.RemoveRuntimeFiles()
			return nil
		}
	}

	// If still running, force kill
	fmt.Println("Server didn't stop gracefully, force killing...")
	if err := stopProcessForce(process); err != nil {
		return fmt.Errorf("failed to force kill process: %w", err)
	}

	_ = fileLock.RemoveRuntimeFiles()
	return nil
}
