package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// Check if process with this PID exists
	// On Unix-like systems, we can check if we can signal the process
	_, err = os.FindProcess(pid)
	if err != nil {
		return false
	}

	return true
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
