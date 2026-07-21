//go:build windows

package daemon

import "syscall"

// daemonSysProcAttr returns the process attributes that detach the daemon
// child from the console on Windows.
func daemonSysProcAttr() *syscall.SysProcAttr {
	// CREATE_NEW_PROCESS_GROUP (0x200) | DETACHED_PROCESS (0x8)
	// This creates a detached process that doesn't inherit the console.
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000208,
	}
}
