//go:build unix

package usernet

import (
	"fmt"
	"os"
	"syscall"
)

// Pair is a connected datagram socket for a guest's card: host is the
// network's Link, guest the end the VM's card is attached to (one Ethernet
// frame per datagram). The host end is non-blocking, so Close wakes a Read
// blocked on it (a blocking descriptor in an *os.File does not).
func Pair() (host, guest *os.File, err error) { return pair(false) }

// pair is Pair, with the guest end non-blocking too when nonblock is set.
func pair(nonblock bool) (host, guest *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("usernet: %w", err)
	}
	for _, fd := range fds {
		syscall.CloseOnExec(fd)
	}
	// Room for bursts of full frames both ways.
	for _, fd := range fds {
		_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 1<<20)
		_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4<<20)
	}
	for i, fd := range fds {
		if i == 1 && !nonblock {
			break
		}
		if err := syscall.SetNonblock(fd, true); err != nil {
			_ = syscall.Close(fds[0])
			_ = syscall.Close(fds[1])
			return nil, nil, fmt.Errorf("usernet: %w", err)
		}
	}
	return os.NewFile(uintptr(fds[0]), "usernet-host"), os.NewFile(uintptr(fds[1]), "usernet-guest"), nil
}
