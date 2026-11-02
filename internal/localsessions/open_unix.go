//go:build !windows

package localsessions

import (
	"os"
	"syscall"
)

// Nonblocking open makes a regular-file-to-FIFO race fail at descriptor
// validation instead of blocking a worker indefinitely.
func openReadOnly(root *os.Root, rel string) (*os.File, error) {
	return root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
