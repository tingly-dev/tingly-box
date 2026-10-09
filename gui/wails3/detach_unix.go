//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in its own session so it survives the launching process and
// its controlling terminal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
