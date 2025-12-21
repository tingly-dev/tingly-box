package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PIDManager manages server PID files
type PIDManager struct {
	pidFile     string
	createdTime int64 // Store when PID file was created to detect stale files
}

// NewPIDManager creates a new PID manager
func NewPIDManager(configDir string) *PIDManager {
	// Use current working directory for PID file
	return &PIDManager{
		pidFile: filepath.Join(configDir, "tingly-server.pid"),
	}
}

// CreatePIDFile creates a PID file with current process ID
func (pm *PIDManager) CreatePIDFile() error {
	pid := os.Getpid()
	pm.createdTime = time.Now().Unix()

	// Write PID with timestamp to help detect stale files
	content := fmt.Sprintf("%d\n%d", pid, pm.createdTime)

	// Use exclusive create to avoid race conditions
	file, err := os.OpenFile(pm.pidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("PID file already exists, server may already be running")
		}
		return fmt.Errorf("failed to create PID file: %w", err)
	}
	defer file.Close()

	fmt.Printf("Save PID file: %s (PID: %d)\n", pm.pidFile, pid)
	if _, err = file.WriteString(content); err != nil {
		file.Close()
		_ = os.Remove(pm.pidFile) // Clean up on error
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	return file.Close()
}

// RemovePIDFile removes the PID file
func (pm *PIDManager) RemovePIDFile() error {
	return os.Remove(pm.pidFile)
}

// IsRunning checks if PID file exists and process is running
func (pm *PIDManager) IsRunning() bool {
	// Check if PID file exists
	fileInfo, err := os.Stat(pm.pidFile)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}

	// Check if PID file is too old (stale)
	// PID files older than 24 hours are considered stale
	if time.Since(fileInfo.ModTime()) > 24*time.Hour {
		// Try to remove stale PID file
		_ = os.Remove(pm.pidFile)
		return false
	}

	// Read PID from file
	pidData, err := os.ReadFile(pm.pidFile)
	if err != nil {
		return false
	}

	lines := strings.Split(strings.TrimSpace(string(pidData)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return false
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return false
	}

	// Additional check: verify the process started after the PID file was created
	if !pm.verifyProcessStartTime(pid, fileInfo.ModTime()) {
		return false
	}

	// Platform-specific process verification
	if runtime.GOOS == "windows" {
		return pm.isProcessRunningWindows(pid)
	}
	return pm.isProcessRunningUnix(pid)
}

// verifyProcessStartTime checks if the process started after PID file was created
func (pm *PIDManager) verifyProcessStartTime(pid int, pidFileTime time.Time) bool {
	// Get process creation time
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix-like systems, we can check the process start time through /proc
	if runtime.GOOS != "windows" {
		statPath := fmt.Sprintf("/proc/%d/stat", pid)
		if statData, err := os.ReadFile(statPath); err == nil {
			fields := strings.Fields(string(statData))
			if len(fields) > 21 {
				// Field 22 in /proc/[pid]/stat is the process start time in clock ticks
				startTicks, err := strconv.ParseInt(fields[21], 10, 64)
				if err == nil {
					// Convert clock ticks to milliseconds since boot
					// This is a simplified check - in production, you might want to get boot time
					processStartTime := time.Unix(startTicks/100, 0)
					return processStartTime.After(pidFileTime.Add(-time.Minute))
				}
			}
		}
	}

	// Fallback: just check if process exists using signal 0
	// This is a less accurate but more portable check
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// If we can't signal the process, it likely doesn't exist
		return false
	}
	return true
}

// isProcessRunningUnix checks if process is running on Unix-like systems
func (pm *PIDManager) isProcessRunningUnix(pid int) bool {
	// On Unix-like systems, we can send signal 0 to check if process exists
	// signal 0 doesn't actually send a signal but performs error checking
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process is alive
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Check for specific error types using errors.Is
		if errors.Is(err, syscall.ESRCH) {
			return false
		}
		// EPERM means process exists but we don't have permission to signal it
		// In this case, the process is still running
		if errors.Is(err, syscall.EPERM) {
			return true
		}
		// Other errors likely mean process is not accessible
		return false
	}

	return true
}

// isProcessRunningWindows checks if process is running on Windows
func (pm *PIDManager) isProcessRunningWindows(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Windows, we use a non-blocking approach to check process status
	// Create a channel to receive the result
	result := make(chan bool, 1)

	go func() {
		// Try to get process exit code with timeout
		// This is a safer alternative to process.Wait() which can block
		defer func() {
			if r := recover(); r != nil {
				result <- false
			}
		}()

		// Use signal to check if process exists (Windows implementation)
		// This doesn't actually send a signal but checks process existence
		err := process.Signal(syscall.Signal(0))
		if err == nil {
			result <- true
			return
		}

		// If we get an access denied error, the process exists
		if err.Error() == "access denied" {
			result <- true
			return
		}

		result <- false
	}()

	// Wait for result with timeout
	select {
	case res := <-result:
		return res
	case <-time.After(1 * time.Second):
		// Timeout means we couldn't determine the process status
		// Assume it's running to be safe
		return true
	}
}

// GetPID returns the PID from file
func (pm *PIDManager) GetPID() (int, error) {
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		return 0, fmt.Errorf("PID file does not exist")
	}

	pidData, err := os.ReadFile(pm.pidFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	// Parse only the first line which contains the PID
	lines := strings.Split(strings.TrimSpace(string(pidData)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0, fmt.Errorf("PID file is empty or invalid")
	}

	return strconv.Atoi(lines[0])
}

// GetPIDFilePath returns the PID file path
func (pm *PIDManager) GetPIDFilePath() string {
	return pm.pidFile
}
