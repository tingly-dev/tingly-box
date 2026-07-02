//go:build windows

package command

import (
	"os"
	"os/exec"
)

// execReplace approximates process replacement on Windows, which has no
// true exec(). It starts binPath as a child, waits for it to finish, and
// exits the current process with the child's exit code so tingly-box does
// not remain resident once the child is running.
func execReplace(binPath string, args []string, env []string) error {
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
