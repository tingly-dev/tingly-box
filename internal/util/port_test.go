package util

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestIsPortAvailable(t *testing.T) {
	// Use a high port number that's likely available
	port := 12580

	// Debug: Check port with detailed info
	available, info := IsPortAvailableWithInfo("", port)
	t.Logf("Initial check for port %d: available=%v, info=%s", port, available, info)

	if !available {
		t.Logf("WARNING: Port %d was already occupied before test!", port)
		t.Logf("This means another process is using port 12580")
		t.Logf("Try running: lsof -i :12580  to find the process")
	}

	// Occupy the port and test again
	listener, err := net.Listen("tcp", ":12580")
	if err != nil {
		t.Fatalf("Failed to occupy port for testing: %v", err)
	}
	defer listener.Close()
	t.Logf("Successfully occupied port %d with test listener", port)

	// Now check again while port is occupied
	available, info = IsPortAvailableWithInfo("", port)
	t.Logf("Check while port is occupied: available=%v, info=%s", available, info)

	if available {
		t.Errorf("ERROR: Port %d is occupied but IsPortAvailable returned true!", port)
		t.Errorf("This should NOT happen - net.Listen should have failed")
	} else {
		t.Logf("SUCCESS: Port %d correctly detected as occupied", port)
	}
}

func TestGetAvailablePort(t *testing.T) {
	// Find an available port in a range
	port, err := GetAvailablePort(15000, 15100)
	if err != nil {
		t.Errorf("Failed to get available port: %v", err)
	}
	if port < 15000 || port > 15100 {
		t.Errorf("Port %d is out of range [15000, 15100]", port)
	}
}

func TestWaitForPortAvailable(t *testing.T) {
	port := 15433

	// Occupy the port
	listener, err := net.Listen("tcp", ":15433")
	if err != nil {
		t.Fatalf("Failed to occupy port for testing: %v", err)
	}

	// Start a goroutine to release the port after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		listener.Close()
	}()

	// Should succeed within 200ms
	err = WaitForPortAvailable(port, 200*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForPortAvailable failed: %v", err)
	}
}

// TestMainPortDetection is a manual test for debugging port detection
// Run with: go test -v -run TestMainPortDetection ./internal/util/
func TestMainPortDetection(t *testing.T) {
	port := 12580

	t.Logf("Testing port %d detection...", port)

	// First check - should be available
	available := IsPortAvailable(port)
	t.Logf("First check (should be true): %v", available)

	// Occupy the port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("Failed to occupy port: %v", err)
	}
	t.Logf("Port %d is now occupied", port)

	// Second check - should NOT be available
	available = IsPortAvailable(port)
	t.Logf("Second check while occupied (should be false): %v", available)

	if available {
		t.Errorf("ERROR: Port %d is occupied but IsPortAvailable returned true!", port)
	}

	// Wait a bit then close
	time.Sleep(100 * time.Millisecond)
	listener.Close()
	t.Logf("Port %d is now released", port)

	// Third check - should be available again
	available = IsPortAvailable(port)
	t.Logf("Third check after release (should be true): %v", available)

	if !available {
		t.Errorf("ERROR: Port %d is released but IsPortAvailable returned false!", port)
	}
}
