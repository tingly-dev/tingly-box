//go:build windows

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/windows"
)

// FileLock manages exclusive file locking for single-instance enforcement.
// The lock is automatically released when the process dies, even if it crashes.
// It also stores the current process PID for signal-based shutdown.
type FileLock struct {
	lockFile string
	file     *os.File
	pid      int
}

// NewFileLock creates a new file lock instance.
// The lock file will be created in the specified config directory.
func NewFileLock(configDir string) *FileLock {
	return &FileLock{
		lockFile: filepath.Join(configDir, "tingly-server.lock"),
	}
}

// TryLock attempts to acquire the file lock.
// Returns an error if the lock is already held by another process.
// The lock file remains on disk but is unlocked when this process dies.
// On success, stores the current process PID in the lock file for shutdown signals.
func (fl *FileLock) TryLock() error {
	var err error
	fl.file, err = os.OpenFile(fl.lockFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire exclusive lock with immediate failure if locked
	flag := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(fl.file.Fd()),
		flag,
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		&overlapped,
	)
	if err != nil {
		fl.file.Close()
		fl.file = nil
		return fmt.Errorf("lock already held: server may already be running")
	}

	// Store current PID for stop command
	fl.pid = os.Getpid()
	if _, err := fl.file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek lock file: %w", err)
	}
	if _, err := fl.file.WriteString(strconv.Itoa(fl.pid) + "\n"); err != nil {
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}

	return nil
}

// Unlock releases the file lock.
// Safe to call multiple times; subsequent calls are no-ops.
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}

	// Release the Windows file lock
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(
		windows.Handle(fl.file.Fd()),
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		&overlapped,
	)

	// Close the file handle
	closeErr := fl.file.Close()
	fl.file = nil

	// Remove the lock file (optional, keeps directory clean)
	_ = os.Remove(fl.lockFile)

	if closeErr != nil {
		return fmt.Errorf("failed to close lock file: %w", closeErr)
	}

	return nil
}

// IsLocked checks if the lock is currently held by another process.
// Returns false if the lock is available or if this process holds it.
func (fl *FileLock) IsLocked() bool {
	file, err := os.OpenFile(fl.lockFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false
	}
	defer file.Close()

	flag := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		flag,
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		&overlapped,
	)
	if err != nil {
		// Failed to acquire lock means someone else holds it
		return true
	}
	// We acquired the lock, immediately release it
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
	return false
}

// GetLockFilePath returns the lock file path for debugging purposes.
func (fl *FileLock) GetLockFilePath() string {
	return fl.lockFile
}

// GetPID returns the PID stored in the lock file.
// Returns error if the lock file doesn't exist or contains invalid data.
func (fl *FileLock) GetPID() (int, error) {
	data, err := os.ReadFile(fl.lockFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read lock file: %w", err)
	}

	if len(data) == 0 {
		return 0, fmt.Errorf("lock file is empty")
	}

	// Find newline and take first line only
	pidStr := string(data)
	for i, c := range pidStr {
		if c == '\n' {
			pidStr = pidStr[:i]
			break
		}
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in lock file: %w", err)
	}

	return pid, nil
}
