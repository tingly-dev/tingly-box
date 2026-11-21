//go:build windows

package live

import (
	"os/exec"
	"time"
)

func ownGroup(*exec.Cmd) {}

// terminate stops the process; on Windows its children are not tracked.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}
