//go:build !windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Daemonize detaches the process from the terminal and runs in the background
// This works on Unix-like systems (Linux, macOS)
func Daemonize() error {
	// Check if we're already the child process
	if IsDaemonProcess() {
		return nil
	}

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get original arguments
	args := os.Args[1:]

	// Set environment variable to mark the child as daemon
	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), "_TINGLY_BOX_DAEMON=1")

	// Redirect stdin, stdout, stderr
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set process attributes to detach from terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}

	// Start the daemonized process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Parent process exits
	os.Exit(0)
	return nil
}
