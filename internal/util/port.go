package util

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// IsPortAvailable checks if a port is available for listening
func IsPortAvailable(port int) bool {
	address := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		// Port is not available
		return false
	}
	// Successfully bound to port, now close and report as available
	listener.Close()
	return true
}

// IsPortAvailableWithInfo checks if a port is available and returns detailed info
func IsPortAvailableWithInfo(host string, port int) (available bool, info string) {
	address := host + ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false, fmt.Sprintf("Failed to bind: %v", err)
	}
	// Get local address for debugging
	localAddr := listener.Addr().String()
	listener.Close()
	return true, fmt.Sprintf("Successfully bound to %s then closed", localAddr)
}

// GetAvailablePort returns an available port in the range [min, max]
func GetAvailablePort(min, max int) (int, error) {
	for port := min; port <= max; port++ {
		if IsPortAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range [%d, %d]", min, max)
}

// WaitForPortAvailable waits until the port becomes available or timeout occurs
func WaitForPortAvailable(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsPortAvailable(port) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %d not available after %v", port, timeout)
}
