//go:build !windows

package command

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/lock"
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

	// Wait for process to exit
	for i := 0; i < 5; i++ { // Wait up to 5 seconds
		if !fileLock.IsLocked() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	// If still running, force kill
	fmt.Println("Server didn't stop gracefully, force killing...")
	if err := stopProcessForce(process); err != nil {
		return fmt.Errorf("failed to force kill process: %w", err)
	}

	return nil
}
