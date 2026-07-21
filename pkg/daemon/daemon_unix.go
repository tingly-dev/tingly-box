//go:build !windows

package daemon

import "syscall"

// daemonSysProcAttr returns the process attributes that detach the daemon
// child from the terminal on Unix-like systems (Linux, macOS).
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}
}
