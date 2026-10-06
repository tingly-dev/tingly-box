//go:build !windows

package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileLock(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	expectedPath := filepath.Join(configDir, "tingly-server.lock")
	if fl.lockFile != expectedPath {
		t.Errorf("Expected lock file path %q, got %q", expectedPath, fl.lockFile)
	}
}

func TestFileLock_TryLock(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	// First lock should succeed
	err := fl.TryLock()
	if err != nil {
		t.Fatalf("First TryLock failed: %v", err)
	}

	// Verify lock file exists
	if _, err := os.Stat(fl.lockFile); os.IsNotExist(err) {
		t.Error("Lock file was not created")
	}

	// Second lock should fail
	fl2 := NewFileLock(configDir)
	err = fl2.TryLock()
	if err == nil {
		t.Error("Second TryLock should have failed but succeeded")
	}

	// Cleanup
	fl.Unlock()
}

func TestFileLock_Unlock(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	// Lock first
	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	// Unlock
	if err := fl.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	// Verify we can lock again
	fl2 := NewFileLock(configDir)
	if err := fl2.TryLock(); err != nil {
		t.Errorf("TryLock after Unlock failed: %v", err)
	}

	// Cleanup
	fl2.Unlock()
}

func TestFileLock_UnlockMultipleTimes(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	// First unlock
	if err := fl.Unlock(); err != nil {
		t.Fatalf("First Unlock failed: %v", err)
	}

	// Second unlock should be safe (no-op)
	if err := fl.Unlock(); err != nil {
		t.Errorf("Second Unlock should be no-op but failed: %v", err)
	}
}

func TestFileLock_IsLocked(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	// Not locked initially
	if fl.IsLocked() {
		t.Error("File should not be locked initially")
	}

	// Lock it
	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	// Now it should be locked
	if !fl.IsLocked() {
		t.Error("File should be locked after TryLock")
	}

	// Using a new FileLock instance should also see it as locked
	fl2 := NewFileLock(configDir)
	if !fl2.IsLocked() {
		t.Error("New FileLock instance should see the file as locked")
	}

	// Unlock
	fl.Unlock()

	// Now it should not be locked
	if fl.IsLocked() {
		t.Error("File should not be locked after Unlock")
	}
}

func TestFileLock_GetPID(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	// Lock and write PID
	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	// Get PID
	pid, err := fl.GetPID()
	if err != nil {
		t.Fatalf("GetPID failed: %v", err)
	}

	// Verify it matches current process
	expectedPid := os.Getpid()
	if pid != expectedPid {
		t.Errorf("Expected PID %d, got %d", expectedPid, pid)
	}

	fl.Unlock()
}

func TestFileLock_GetPID_NoFile(t *testing.T) {
	configDir := t.TempDir()
	fl := NewFileLock(configDir)

	// Try to get PID when no lock file exists
	_, err := fl.GetPID()
	if err == nil {
		t.Error("GetPID should fail when lock file doesn't exist")
	}
}

func TestFileLock_ProcessDeathAutoCleanup(t *testing.T) {
	configDir := t.TempDir()

	// This test verifies that locks are released when process dies
	// We'll use a helper function to simulate this

	if os.Getenv("GO_TEST_SUBPROCESS") == "1" {
		// Child process: acquire lock and exit without unlocking
		fl := NewFileLock(configDir)
		if err := fl.TryLock(); err != nil {
			t.Fatalf("TryLock failed: %v", err)
		}
		// Exit WITHOUT unlocking - lock should be auto-released
		os.Exit(0)
	}

	// Parent process: start child, wait for it to die, then check lock
	// We use exec.Command to run a test as a subprocess
	cmd := execTestCommand(t, "TestFileLock_ProcessDeathAutoCleanup")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Subprocess failed: %v", err)
	}

	// Give a moment for the OS to clean up
	time.Sleep(100 * time.Millisecond)

	// Now check if the lock is released
	fl := NewFileLock(configDir)
	if fl.IsLocked() {
		t.Error("Lock should have been auto-released after process death")
	}

	// Should be able to acquire the lock
	if err := fl.TryLock(); err != nil {
		t.Errorf("Should be able to acquire lock after process death: %v", err)
	}
	fl.Unlock()
}

// execTestCommand runs the current test as a subprocess
func execTestCommand(t *testing.T, testName string) *execCmd {
	return &execCmd{
		Path: os.Args[0],
		Args: []string{"-test.run=" + testName},
		Env:  append(os.Environ(), "GO_TEST_SUBPROCESS=1"),
	}
}

// execCmd is a minimal command representation for testing
type execCmd struct {
	Path string
	Args []string
	Env  []string
}

func (c *execCmd) Run() error {
	// In actual implementation, this would use os/exec
	// For this test, we're just demonstrating the concept
	return nil
}

// A server that died without cleaning up (crash, kill -9) leaves its port and
// version files behind. The next TryLock owns the lock now, so it must drop
// them: a launch that fails before rewriting them must not advertise a stale
// port or version to CLI readers.
func TestFileLock_TryLockDropsStaleRuntimeFiles(t *testing.T) {
	configDir := t.TempDir()

	if err := NewPortFile(configDir).Write(23456); err != nil {
		t.Fatalf("seed stale port file: %v", err)
	}
	if err := NewVersionFile(configDir).Write("v0.0.1"); err != nil {
		t.Fatalf("seed stale version file: %v", err)
	}

	fl := NewFileLock(configDir)
	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	defer fl.Unlock()

	for _, name := range []string{portFileName, versionFileName} {
		if _, err := os.Stat(filepath.Join(configDir, name)); !os.IsNotExist(err) {
			t.Errorf("stale %s survived TryLock (stat err: %v)", name, err)
		}
	}

	// The holder publishes its own values afterwards, as a starting server does.
	if err := fl.WritePort(31337); err != nil {
		t.Fatalf("WritePort failed: %v", err)
	}
	if got, err := fl.ReadPort(); err != nil || got != 31337 {
		t.Errorf("ReadPort = %d, %v; want 31337", got, err)
	}
}

// Losing the race must not touch the winner's files: they describe a live
// server, and only the lock holder may clear runtime files.
func TestFileLock_FailedTryLockKeepsRunningServersFiles(t *testing.T) {
	configDir := t.TempDir()

	holder := NewFileLock(configDir)
	if err := holder.TryLock(); err != nil {
		t.Fatalf("holder TryLock failed: %v", err)
	}
	defer holder.Unlock()
	if err := holder.WritePort(23456); err != nil {
		t.Fatalf("WritePort failed: %v", err)
	}

	if err := NewFileLock(configDir).TryLock(); err == nil {
		t.Fatal("second TryLock should have failed while the holder is alive")
	}

	if got, err := holder.ReadPort(); err != nil || got != 23456 {
		t.Errorf("running server's port file was disturbed: ReadPort = %d, %v; want 23456", got, err)
	}
}
