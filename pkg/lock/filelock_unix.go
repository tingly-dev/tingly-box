//go:build !windows

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"
)

// globalLockRegistry keeps FileLock references from being garbage collected.
// On Unix, if the FileLock struct is GC'd, the file descriptor is closed
// and the flock() lock is released even though the process is still running.
var (
	globalLockRegistryMu sync.Mutex
	globalLockRegistry   = make(map[*FileLock]bool)
)

// FileLock manages exclusive file locking for single-instance enforcement.
// The lock is automatically released when the process dies, even if it crashes.
// It also stores the current process PID for signal-based shutdown.
type FileLock struct {
	lockFile string
	file     *os.File
	pid      int
	portFile *PortFile
}

// NewFileLock creates a new file lock instance.
// The lock file will be created in the specified config directory.
func NewFileLock(configDir string) *FileLock {
	return &FileLock{
		lockFile: filepath.Join(configDir, "tingly-server.lock"),
		portFile: NewPortFile(configDir),
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

	// Try to acquire exclusive non-blocking lock
	err = unix.Flock(int(fl.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		fl.file.Close()
		fl.file = nil
		return fmt.Errorf("lock already held: server may already be running")
	}

	// Store current PID for stop command
	fl.pid = os.Getpid()
	if _, err := fl.file.Seek(0, 0); err != nil {
		fl.releaseAndClose()
		return fmt.Errorf("failed to seek lock file: %w", err)
	}
	if _, err := fl.file.WriteString(strconv.Itoa(fl.pid) + "\n"); err != nil {
		fl.releaseAndClose()
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}

	// Register this lock globally to prevent garbage collection
	// On Unix, if the FileLock is GC'd, the file is closed and lock is released
	globalLockRegistryMu.Lock()
	globalLockRegistry[fl] = true
	globalLockRegistryMu.Unlock()

	return nil
}

// releaseAndClose drops the flock and closes the file handle, leaving the
// lock available for the next caller. Used on post-acquire error paths so a
// failed TryLock never leaks the fd or keeps the lock held.
func (fl *FileLock) releaseAndClose() {
	_ = unix.Flock(int(fl.file.Fd()), unix.LOCK_UN)
	_ = fl.file.Close()
	fl.file = nil
}

// Unlock releases the file lock.
// Safe to call multiple times; subsequent calls are no-ops.
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}

	// Unregister from global registry
	globalLockRegistryMu.Lock()
	delete(globalLockRegistry, fl)
	globalLockRegistryMu.Unlock()

	// Release the flock
	_ = unix.Flock(int(fl.file.Fd()), unix.LOCK_UN)

	// Close the file handle
	closeErr := fl.file.Close()
	fl.file = nil

	// Remove the lock file and the associated runtime port file (both are
	// runtime artifacts of this lock; keeps the config directory clean).
	_ = os.Remove(fl.lockFile)
	_ = fl.portFile.Remove()

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

	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		// Failed to acquire lock means someone else holds it
		return true
	}
	// We acquired the lock, immediately release it
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return false
}

// GetPID returns the PID stored in the lock file.
// Returns error if the lock file doesn't exist or contains invalid data.
func (fl *FileLock) GetPID() (int, error) {
	return readPIDFile(fl.lockFile, "lock file")
}
