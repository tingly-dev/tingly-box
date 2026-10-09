//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a new process group, detached from this console.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008} // CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
}
