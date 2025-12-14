package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// PIDManager manages server PID files
type PIDManager struct {
	pidFile string
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
	fmt.Printf("Save PID file: %s\n", pm.pidFile)
	return os.WriteFile(pm.pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// RemovePIDFile removes the PID file
func (pm *PIDManager) RemovePIDFile() error {
	return os.Remove(pm.pidFile)
}

// IsRunning checks if PID file exists and process is running
func (pm *PIDManager) IsRunning() bool {
	// Check if PID file exists
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		return false
	}

	// Read PID from file
	pidData, err := os.ReadFile(pm.pidFile)
	if err != nil {
		return false
	}

	pidStr := strings.TrimSpace(string(pidData))
	if pidStr == "" {
		return false
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// Platform-specific process verification
	if runtime.GOOS == "windows" {
		return pm.isProcessRunningWindows(pid)
	}
	return pm.isProcessRunningUnix(pid)
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
		// ESRCH means no such process
		if err == syscall.ESRCH {
			return false
		}
		// EPERM means process exists but we don't have permission to signal it
		// In this case, the process is still running
		if err == syscall.EPERM {
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

	// On Windows, we can check if process is still running by checking if we can
	// wait on it without blocking
	// This is not perfect but better than nothing
	// Note: Process.IsRunning() is not available in older Go versions
	// We'll use a simple approach by checking the exit code
	state, err := process.Wait()
	if err != nil {
		// If we can't wait, assume process is running
		// This is a conservative approach
		return true
	}

	// If process has already exited, state.Exited() will be true
	// If we get here immediately after FindProcess, it likely means
	// the process is not running anymore
	return !state.Exited()
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

	pidStr := strings.TrimSpace(string(pidData))
	return strconv.Atoi(pidStr)
}

// GetPIDFilePath returns the PID file path
func (pm *PIDManager) GetPIDFilePath() string {
	return pm.pidFile
}
