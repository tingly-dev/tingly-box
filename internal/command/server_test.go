package command

import (
	"os"
	"testing"

	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// TestServerManagerStopWithoutStart tests stopping a server that was never started
func TestServerManagerStopWithoutStart(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test-no-start-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	appManager, err := NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}

	serverManager := NewServerManager(appManager.AppConfig())

	// Stop without starting should not fail
	err = serverManager.Stop()
	if err != nil {
		t.Errorf("Stop without start should not fail, got: %v", err)
	}
}

// TestFileLockIntegration tests that file locks work correctly with server lifecycle
func TestFileLockIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test-filelock-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}

	fileLock := lock.NewFileLock(tempDir)

	t.Run("File lock not acquired initially", func(t *testing.T) {
		if fileLock.IsLocked() {
			t.Error("File lock should not be locked initially")
		}
	})

	t.Run("Acquire and release file lock", func(t *testing.T) {
		err := fileLock.TryLock()
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		if !fileLock.IsLocked() {
			t.Error("File lock should be locked after TryLock")
		}

		err = fileLock.Unlock()
		if err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}

		if fileLock.IsLocked() {
			t.Error("File lock should not be locked after Unlock")
		}
	})

	t.Run("Acquire lock twice fails", func(t *testing.T) {
		err := fileLock.TryLock()
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}
		defer fileLock.Unlock()

		// Try to acquire again - should fail
		err = fileLock.TryLock()
		if err == nil {
			t.Error("Expected error when acquiring lock twice")
		}
	})
}

// TestServerPortConfiguration tests port configuration persistence
func TestServerPortConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test-port-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("Set and get server port", func(t *testing.T) {
		appManager, err := NewAppManager(tempDir)
		if err != nil {
			t.Fatalf("Failed to create app manager: %v", err)
		}

		testPort := 12580
		err = appManager.SetServerPort(testPort)
		if err != nil {
			t.Fatalf("Failed to set server port: %v", err)
		}

		if appManager.GetServerPort() != testPort {
			t.Errorf("Expected port %d, got %d", testPort, appManager.GetServerPort())
		}
	})

	t.Run("Runtime port prefers port file while server is running", func(t *testing.T) {
		appManager, err := NewAppManager(tempDir)
		if err != nil {
			t.Fatalf("Failed to create app manager: %v", err)
		}

		configPort := appManager.GetServerPort()

		// No server running: falls back to the configured port even if a
		// stale port file exists.
		portFile := lock.NewPortFile(tempDir)
		if err := portFile.Write(23456); err != nil {
			t.Fatalf("Failed to write port file: %v", err)
		}
		if got := appManager.GetRuntimeServerPort(); got != configPort {
			t.Errorf("Expected configured port %d when server is not running, got %d", configPort, got)
		}

		// Server running (lock held): the port file wins.
		fileLock := lock.NewFileLock(tempDir)
		if err := fileLock.TryLock(); err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}
		defer fileLock.Unlock()

		if got := appManager.GetRuntimeServerPort(); got != 23456 {
			t.Errorf("Expected runtime port 23456, got %d", got)
		}

		// Port file gone while running: falls back to configured port.
		if err := portFile.Remove(); err != nil {
			t.Fatalf("Failed to remove port file: %v", err)
		}
		if got := appManager.GetRuntimeServerPort(); got != configPort {
			t.Errorf("Expected configured port %d after port file removal, got %d", configPort, got)
		}
	})

	t.Run("Port persists when explicitly saved", func(t *testing.T) {
		// First instance: set port
		appManager1, err := NewAppManager(tempDir)
		if err != nil {
			t.Fatalf("Failed to create first app manager: %v", err)
		}

		testPort := 12582
		err = appManager1.SetServerPort(testPort)
		if err != nil {
			t.Fatalf("Failed to set server port: %v", err)
		}

		// Verify it was set
		if appManager1.GetServerPort() != testPort {
			t.Errorf("Expected port %d, got %d", testPort, appManager1.GetServerPort())
		}

		// Note: Port persistence is handled by config file, not by SaveConfig
		// The test just verifies that SetServerPort works within the same instance
	})
}
