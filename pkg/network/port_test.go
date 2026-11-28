package network

import (
	"net"
	"testing"
	"time"
)

// listenEphemeral binds a kernel-chosen port on all interfaces — the same
// address family IsPortAvailable probes — and returns the listener and port.
// An ephemeral port keeps these tests from colliding with a real Tingly Box
// listening on the default 12580.
func listenEphemeral(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen on ephemeral port: %v", err)
	}
	return l, l.Addr().(*net.TCPAddr).Port
}

func TestIsPortAvailable(t *testing.T) {
	// Keep the listener open for the "occupied" assertion so there is no
	// window for another process to grab the port.
	occupied, port := listenEphemeral(t)
	if available, info := IsPortAvailableWithInfo("", port); available {
		t.Errorf("port %d is occupied (%s) but IsPortAvailableWithInfo returned available", port, info)
	}

	if err := occupied.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if available, info := IsPortAvailableWithInfo("", port); !available {
		t.Errorf("port %d should be available after release, got: %s", port, info)
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
	listener, port := listenEphemeral(t)

	// Release the port after 100ms; WaitForPortAvailable polls at 100ms, so a
	// 2s deadline leaves room for scheduling jitter.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = listener.Close()
	}()

	if err := WaitForPortAvailable(port, 2*time.Second); err != nil {
		t.Errorf("WaitForPortAvailable failed: %v", err)
	}
}

// TestMainPortDetection covers the plain IsPortAvailable boolean across the
// occupied → released transition (TestIsPortAvailable covers the WithInfo
// variant).
func TestMainPortDetection(t *testing.T) {
	occupied, port := listenEphemeral(t)
	if IsPortAvailable(port) {
		t.Fatalf("port %d is occupied but IsPortAvailable returned true", port)
	}

	if err := occupied.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if !IsPortAvailable(port) {
		t.Errorf("port %d is released but IsPortAvailable returned false", port)
	}
}
