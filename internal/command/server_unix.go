//go:build !windows

package command

import (
	"errors"
	"fmt"
	"os"
	"syscall"

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

// processGone reports whether a signal failed only because the process is
// already gone. FindProcess on Unix never fails for a missing PID, so this is
// how "already exited" is told apart from a real error.
func processGone(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

// stopServerWithFileLock stops the running server using the file lock (Unix version).
//
// It is bounded end to end: SIGTERM, wait up to stopGracePeriod for the lock
// to be released; then SIGKILL, wait up to stopForcePeriod. If the process
// still holds the lock it returns an explicit error instead of pretending the
// server stopped — `restart` depends on this being truthful.
func stopServerWithFileLock(fileLock *lock.FileLock) error {
	if !fileLock.IsLocked() {
		// Nothing running; drop any stale runtime files.
		_ = fileLock.RemoveRuntimeFiles()
		return nil
	}

	// Get PID from lock file
	pid, err := fileLock.GetPID()
	if err != nil {
		return fmt.Errorf("server holds the lock but its PID is unreadable (%v); stop it manually", err)
	}
	// Read the live port before stopping so we can confirm the socket is
	// released; stopping removes the port file.
	port, _ := fileLock.ReadPort()

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find server process %d: %w", pid, err)
	}

	// Send SIGTERM for graceful shutdown
	if err := stopProcessGracefully(process); err != nil {
		if !processGone(err) {
			return fmt.Errorf("failed to send SIGTERM to server process %d: %w", pid, err)
		}
		return finishStop(fileLock, port)
	}

	// Wait for process to exit. The lock is released only after the server has
	// actually stopped (listener closed, MCP sources disconnected), so
	// graceful shutdown can legitimately take a few seconds.
	if waitLockReleased(fileLock, stopGracePeriod) {
		return finishStop(fileLock, port)
	}

	// If still running, force kill.
	fmt.Printf("Server (pid %d) did not stop within %s, force killing...\n", pid, stopGracePeriod)
	if err := stopProcessForce(process); err != nil {
		if !processGone(err) {
			return fmt.Errorf("failed to SIGKILL server process %d: %w", pid, err)
		}
		return finishStop(fileLock, port)
	}
	if waitLockReleased(fileLock, stopForcePeriod) {
		return finishStop(fileLock, port)
	}
	return fmt.Errorf("server process %d still holds the lock %s after SIGKILL; stop it manually (kill -9 %d)", pid, fileLock.GetLockFilePath(), pid)
}
