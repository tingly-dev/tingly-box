//go:build windows

package command

import (
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/lock"
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

// stopServerWithFileLock stops the running server using the file lock (Windows version).
//
// Bounded end to end: TerminateProcess, then wait up to stopForcePeriod for
// the lock to be released. If it is still held, report an explicit error
// rather than claiming the server stopped.
func stopServerWithFileLock(fileLock *lock.FileLock) error {
	if !fileLock.IsLocked() {
		_ = fileLock.RemoveRuntimeFiles()
		return nil
	}

	// Get PID from lock file
	pid, err := fileLock.GetPID()
	if err != nil {
		return fmt.Errorf("server holds the lock but its PID is unreadable (%v); stop it manually", err)
	}
	port, _ := fileLock.ReadPort()

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find server process %d: %w", pid, err)
	}

	// Kill the process (Windows uses TerminateProcess internally)
	if err := stopProcessGracefully(process); err != nil {
		return fmt.Errorf("failed to stop server process %d: %w", pid, err)
	}

	if waitLockReleased(fileLock, stopForcePeriod) {
		return finishStop(fileLock, port)
	}
	return fmt.Errorf("server process %d still holds the lock %s after being killed; stop it manually", pid, fileLock.GetLockFilePath())
}
