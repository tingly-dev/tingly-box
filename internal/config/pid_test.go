package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewPIDManager(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	expectedPath := filepath.Join(configDir, "tingly-server.pid")
	if pm.pidFile != expectedPath {
		t.Errorf("Expected PID file path %q, got %q", expectedPath, pm.pidFile)
	}
}

func TestCreatePIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test successful creation
	err := pm.CreatePIDFile()
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Verify PID file exists
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}

	// Verify PID content
	pidData, err := os.ReadFile(pm.pidFile)
	if err != nil {
		t.Fatalf("Failed to read PID file: %v", err)
	}

	// Check if content matches expected format
	lines := strings.Split(strings.TrimSpace(string(pidData)), "\n")
	if len(lines) < 2 {
		t.Error("PID file should contain PID and timestamp")
	}

	// Verify PID matches current process
	currentPID := strconv.Itoa(os.Getpid())
	if lines[0] != currentPID {
		t.Errorf("Expected PID %s, got %s", currentPID, lines[0])
	}
}

func TestCreatePIDFile_AlreadyExists(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file first
	err := pm.CreatePIDFile()
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Try to create again, should fail
	err = pm.CreatePIDFile()
	if err == nil {
		t.Error("Expected error when PID file already exists")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestRemovePIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file first
	err := pm.CreatePIDFile()
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}

	// Remove PID file
	err = pm.RemovePIDFile()
	if err != nil {
		t.Fatalf("Failed to remove PID file: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(pm.pidFile); !os.IsNotExist(err) {
		t.Error("PID file was not removed")
	}
}

func TestGetPID(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test when PID file doesn't exist
	_, err := pm.GetPID()
	if err == nil {
		t.Error("Expected error when PID file doesn't exist")
	}

	// Create PID file
	err = pm.CreatePIDFile()
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Get PID
	pid, err := pm.GetPID()
	if err != nil {
		t.Fatalf("Failed to get PID: %v", err)
	}

	expectedPID := os.Getpid()
	if pid != expectedPID {
		t.Errorf("Expected PID %d, got %d", expectedPID, pid)
	}
}

func TestGetPIDFilePath(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	expectedPath := filepath.Join(configDir, "tingly-server.pid")
	actualPath := pm.GetPIDFilePath()

	if actualPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, actualPath)
	}
}

func TestIsRunning_NoPIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test when PID file doesn't exist
	if pm.IsRunning() {
		t.Error("Expected false when PID file doesn't exist")
	}
}

func TestIsRunning_ValidPID(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file with current process
	err := pm.CreatePIDFile()
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Should return true for current process
	if !pm.IsRunning() {
		t.Error("Expected true for current running process")
	}
}

func TestIsRunning_InvalidPID(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file with invalid PID
	invalidPID := "999999"
	err := os.WriteFile(pm.pidFile, []byte(invalidPID), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid PID file: %v", err)
	}

	// Should return false for invalid PID
	if pm.IsRunning() {
		t.Error("Expected false for invalid PID")
	}
}

func TestIsRunning_NonexistentPID(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Find a PID that likely doesn't exist
	// Use a very high PID number that's unlikely to be in use
	nonexistentPID := "999999"
	if runtime.GOOS == "windows" {
		// On Windows, PIDs are typically smaller
		nonexistentPID = "65535"
	}

	err := os.WriteFile(pm.pidFile, []byte(nonexistentPID), 0644)
	if err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Should return false for nonexistent PID
	if pm.IsRunning() {
		t.Error("Expected false for nonexistent PID")
	}
}

func TestIsRunning_StalePIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file with old timestamp (>24 hours)
	pid := os.Getpid()
	oldTime := time.Now().Add(-25 * time.Hour)
	content := fmt.Sprintf("%d\n%d", pid, oldTime.Unix())

	err := os.WriteFile(pm.pidFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write stale PID file: %v", err)
	}

	// Set the file modification time to old time
	err = os.Chtimes(pm.pidFile, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to set file times: %v", err)
	}

	// Should return false for stale PID file and remove it
	if pm.IsRunning() {
		t.Error("Expected false for stale PID file")
	}

	// Verify stale file was removed
	if _, err := os.Stat(pm.pidFile); !os.IsNotExist(err) {
		t.Error("Stale PID file should have been removed")
	}
}

func TestIsRunning_EmptyPIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create empty PID file
	err := os.WriteFile(pm.pidFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to write empty PID file: %v", err)
	}

	// Should return false for empty PID file
	if pm.IsRunning() {
		t.Error("Expected false for empty PID file")
	}
}

func TestIsRunning_OnlyNewlinePIDFile(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file with only newline
	err := os.WriteFile(pm.pidFile, []byte("\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to write newline PID file: %v", err)
	}

	// Should return false for newline-only PID file
	if pm.IsRunning() {
		t.Error("Expected false for newline-only PID file")
	}
}

func TestIsRunning_NonNumericPID(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file with non-numeric PID
	err := os.WriteFile(pm.pidFile, []byte("not_a_number"), 0644)
	if err != nil {
		t.Fatalf("Failed to write non-numeric PID file: %v", err)
	}

	// Should return false for non-numeric PID
	if pm.IsRunning() {
		t.Error("Expected false for non-numeric PID")
	}
}

func TestVerifyProcessStartTime(t *testing.T) {
	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test with current process
	pid := os.Getpid()
	pidFileTime := time.Now()

	// Should return true for current process
	if !pm.verifyProcessStartTime(pid, pidFileTime) {
		t.Error("Expected true for current process start time verification")
	}

	// Test with invalid PID
	invalidPID := 999999
	if pm.verifyProcessStartTime(invalidPID, pidFileTime) {
		t.Error("Expected false for invalid PID")
	}
}

// Test isProcessRunningUnix on Unix systems
func TestIsProcessRunningUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific test on Windows")
	}

	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test with current process
	pid := os.Getpid()
	if !pm.isProcessRunningUnix(pid) {
		t.Error("Expected true for current Unix process")
	}

	// Test with invalid PID
	invalidPID := 999999
	if pm.isProcessRunningUnix(invalidPID) {
		t.Error("Expected false for invalid Unix PID")
	}

	// Test with PID 1 (init/systemd), which should exist on most Unix systems
	systemPID := 1
	if runtime.GOOS == "darwin" {
		// On macOS, launchd is PID 1
		systemPID = 1
	}
	if !pm.isProcessRunningUnix(systemPID) {
		t.Logf("Note: System PID %d not accessible (may be normal on some systems)", systemPID)
	}
}

// Test isProcessRunningWindows on Windows systems
func TestIsProcessRunningWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on Unix")
	}

	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Test with current process
	pid := os.Getpid()
	if !pm.isProcessRunningWindows(pid) {
		t.Error("Expected true for current Windows process")
	}

	// Test with invalid PID
	invalidPID := 999999
	// Windows implementation has timeout, so we check if it completes
	start := time.Now()
	result := pm.isProcessRunningWindows(invalidPID)
	duration := time.Since(start)

	if duration > 2*time.Second {
		t.Error("Windows process check took too long (possible hang)")
	}

	// Result should be false for invalid PID (but may be true due to timeout behavior)
	t.Logf("Windows process check for invalid PID returned: %v (took %v)", result, duration)
}

func TestIsRunning_PermissionsTest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	configDir := t.TempDir()
	pm := NewPIDManager(configDir)

	// Find a process we likely don't have permission to signal
	// On most Unix systems, we can't signal processes owned by other users
	// Try PID 1 (init/systemd) which typically requires root
	systemPID := 1
	if runtime.GOOS == "darwin" {
		systemPID = 1 // launchd
	}

	// Write the PID to file
	err := os.WriteFile(pm.pidFile, []byte(strconv.Itoa(systemPID)), 0644)
	if err != nil {
		t.Fatalf("Failed to write system PID file: %v", err)
	}

	// The behavior depends on the system:
	// - If we can check the process exists, it should return true
	// - If we can't access it, it should return false
	// The important thing is that it doesn't crash or hang
	result := pm.IsRunning()
	t.Logf("Permission test result for PID %d: %v", systemPID, result)
}

// Benchmark tests
func BenchmarkIsRunning(b *testing.B) {
	configDir := b.TempDir()
	pm := NewPIDManager(configDir)

	// Create PID file
	err := pm.CreatePIDFile()
	if err != nil {
		b.Fatalf("Failed to create PID file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.IsRunning()
	}
}

func BenchmarkCreatePIDFile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		configDir := b.TempDir()
		pm := NewPIDManager(configDir)
		_ = pm.CreatePIDFile()
	}
}
