//go:build unix

package microvm

import "syscall"

func accessExec(p string) error { return syscall.Access(p, 1) }
