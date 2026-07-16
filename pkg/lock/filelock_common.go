package lock

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// GetLockFilePath returns the lock file path for debugging purposes.
func (fl *FileLock) GetLockFilePath() string {
	return fl.lockFile
}

// WritePort records the port the running server is listening on. It is a
// runtime artifact tied to this lock's lifetime: written after TryLock and
// removed by Unlock (or RemovePort for a crashed server the stopper cleans up).
func (fl *FileLock) WritePort(port int) error {
	return fl.portFile.Write(port)
}

// ReadPort returns the port recorded by the running server. Callers must gate
// on IsLocked() and treat any error as "port unknown" (fall back to config).
func (fl *FileLock) ReadPort() (int, error) {
	return fl.portFile.Read()
}

// RemovePort deletes the runtime port file. Unlock already does this for the
// lock holder; the stop command uses it to clean up after a server that was
// killed without releasing the lock itself.
func (fl *FileLock) RemovePort() error {
	return fl.portFile.Remove()
}

// readPIDFile reads a PID from the first line of the file at path.
// label names the file kind ("lock file" on Unix, "PID file" on Windows)
// in error messages.
func readPIDFile(path, label string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", label, err)
	}

	if len(data) == 0 {
		return 0, fmt.Errorf("%s is empty", label)
	}

	pidStr, _, _ := strings.Cut(string(data), "\n")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in %s: %w", label, err)
	}

	return pid, nil
}
