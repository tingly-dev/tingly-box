//go:build !windows

package command

import (
	"syscall"
)

// execReplace replaces the current process image with binPath, so the
// tingly-box process does not remain resident for the lifetime of the
// child (avoids accumulating leaked parent processes).
func execReplace(binPath string, args []string, env []string) error {
	argv := append([]string{binPath}, args...)
	return syscall.Exec(binPath, argv, env)
}
