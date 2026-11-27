package macvm

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// hypervisor is what the runtime needs of the machine monitor: on macOS,
// Virtualization.framework (vz_darwin.go); a fake in tests.
type hypervisor interface {
	// Check reports whether VMs can be made here (entitlement, OS version).
	Check() error
	// Limits are the most CPUs and memory (MiB) a VM may have; 0 for no limit known.
	Limits() (cpus, memoryMB int)
	New(cfg vmConfig) (vm, error)
}

// vm is one virtual machine, not yet started.
type vm interface {
	// Listen accepts the guest's vsock connections to the host's port.
	Listen(port uint32) (net.Listener, error)
	Start() error
	// Stop ends the machine at once.
	Stop() error
	// Done is closed when the machine has stopped.
	Done() <-chan struct{}
}

// vmConfig is the machine: boot from Kernel with a read-only Rootfs disk,
// Shares as one virtio-fs device tagged ShareTag, a vsock device, the
// console written to Console, and a network card on Card when it is not nil.
type vmConfig struct {
	Kernel, Rootfs, Cmdline, Console string
	CPUs, MemoryMB                   int
	ShareTag                         string
	Shares                           []share
	Card                             *os.File
	MAC                              string
}

// backend is the hypervisor of this build; nil without one.
var backend hypervisor

// hostMemoryMB is the host's RAM, or 4 GiB when it cannot be read.
func hostMemoryMB() int {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil && n > 0 {
			return int(n >> 20)
		}
	}
	return 4096
}
