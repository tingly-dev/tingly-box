//go:build windows

package command

import (
	"os"
	"os/exec"
)

// execReplace approximates process replacement on Windows, which has no
// true exec(). The child's stdio handles are inherited independently of
// the parent, so it starts binPath as a child and exits tingly-box
// immediately without waiting — the child stays attached to the same
// console (Ctrl+C still reaches it) and keeps running on its own, while
// tingly-box does not remain resident for the claude session.
func execReplace(binPath string, args []string, env []string) error {
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
