//go:build windows

package command

import (
	"os"
	"os/exec"
)

// execReplace approximates process replacement on Windows, which has no
// true exec(). It starts binPath as a child process with the same stdio
// handles, then exits tingly-box immediately without waiting.
//
// In a normal console session, the child remains attached to the same
// console, so Ctrl+C should continue to reach it. This is not identical
// to POSIX exec(), and behavior may differ under job objects, IDEs,
// services, CI, or custom console/process-group setups.
func execReplace(binPath string, args []string, env []string) error {
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if env != nil {
		cmd.Env = env
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}
