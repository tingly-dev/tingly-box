//go:build !windows

package live

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// ownGroup starts cmd in a process group of its own, so stopping it also
// stops what it started (the Claude Code processes of the server): none of
// them outlives the run and writes into its directory after it is removed.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// terminate asks the process group to stop, waits until every member is
// gone, and kills what is left after a grace period.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	group := -cmd.Process.Pid
	_ = syscall.Kill(group, syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(group, 0), syscall.ESRCH) {
			<-done
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(group, syscall.SIGKILL)
	<-done
}
