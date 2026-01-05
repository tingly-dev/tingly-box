//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Daemonize detaches the process from the terminal and runs in the background
// This works on Windows by creating a detached process
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

	// Windows: CREATE_NEW_PROCESS_GROUP (0x200) | DETACHED_PROCESS (0x8)
	// This creates a detached process that doesn't inherit console
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000208, // CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
	}

	// Start the daemonized process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Parent process exits
	os.Exit(0)
	return nil
}
