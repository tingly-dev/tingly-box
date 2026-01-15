//go:build windows

package cli

import (
	"fmt"
	"os"
	"time"

	"tingly-box/internal/util/lock"
)

// stopProcessGracefully attempts to stop a process gracefully on Windows
// On Windows, we use process.Kill() directly since signal handling is not reliable
func stopProcessGracefully(process *os.Process) error {
	// On Windows, process.Signal(syscall.SIGTERM) doesn't work reliably
	// The best approach is to use process.Kill() which calls TerminateProcess
	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	return nil
}

// stopServerWithFileLock stops the running server using the file lock (Windows version)
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

	// Kill the process (Windows uses TerminateProcess internally)
	if err := stopProcessGracefully(process); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait for process to exit and lock to be released
	for i := 0; i < 30; i++ { // Wait up to 30 seconds
		if !fileLock.IsLocked() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	// If lock is still held after process kill, something is wrong
	return fmt.Errorf("process was killed but lock file is still held")
}
