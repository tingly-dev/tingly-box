// Package vmguest is what the virtual machine runtimes (microvm, macvm) share:
// the protocol between the host and a guest's init, and the init itself
// (guest_linux.go), which runs a command in the guest the host describes.
//
// The guest connects to the host (CID 2) over vsock: a control connection
// for the command (Spec) and how it ended (Result), and one each for stdin,
// stdout and stderr; under network allowlist and with Expose, one per
// endpoint of the bridge (Ports). Importing the package makes a binary that
// runs as a guest's init (process 1, with Env set) do so before its main.
package vmguest

import (
	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netproxy"
)

// The vsock ports the guest connects to, on the host.
const (
	PortControl = 1024 + iota
	PortStdin
	PortStdout
	PortStderr
	PortProxy
	// PortExpose + i carries Network.Expose[i].
	PortExpose = 1100
)

// Env, on the kernel command line, tells a binary it is a guest's init (the
// kernel hands such words to init as its environment).
const Env = "TB_SANDBOX_GUEST"

// Spec is what the host sends the guest on the control connection.
type Spec struct {
	Args   []string `json:"args"`
	Env    []string `json:"env"`
	Dir    string   `json:"dir"`
	UID    int      `json:"uid"`
	GID    int      `json:"gid"`
	Proxy  bool     `json:"proxy,omitempty"`
	Expose []int    `json:"expose,omitempty"`
	// Net sets up the guest's network card (internet).
	Net *Net `json:"net,omitempty"`
	// Limits, when set, are the command's cgroup in the guest.
	Limits *sandbox.Resources `json:"limits,omitempty"`

	// The guest builds its own view when the host cannot (macvm). Root
	// makes the read-only root writable with an overlay in the guest's
	// memory, for the mount points; then the guest mounts Fresh and Mounts
	// in the order of their paths, so the longest prefix wins.
	Root bool `json:"root,omitempty"`
	// Fresh are empty directories in the guest's memory (home, /tmp, /run).
	Fresh []string `json:"fresh,omitempty"`
	// Mounts are paths of the virtio-fs share Tag, put at their path.
	Tag    string  `json:"tag,omitempty"`
	Mounts []Mount `json:"mounts,omitempty"`
	// ResolvConf, when set, is written to /etc/resolv.conf.
	ResolvConf string `json:"resolv_conf,omitempty"`
	// User, when set, names the host user in the guest's /etc/passwd.
	User string `json:"user,omitempty"`
	// Foreign says the guest's system is not the host's: a command given as
	// a host path the guest cannot run is looked up by its base name.
	Foreign bool `json:"foreign,omitempty"`
}

// Mount is a file or directory of the share, seen at Path.
type Mount struct {
	Source   string `json:"source"` // relative to the share's root
	Path     string `json:"path"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// Net is the guest's network card: its address and gateway.
type Net struct {
	Addr    string `json:"addr"`
	Gateway string `json:"gateway"`
}

// Result is what the guest sends back when the command has ended, or could
// not start.
type Result struct {
	Exit   int    `json:"exit"`
	Signal int    `json:"signal,omitempty"`
	OOM    bool   `json:"oom,omitempty"`
	Error  string `json:"error,omitempty"`
}

// End is how the command ended, as Proc.Wait returns it: nil for a zero exit.
func (r Result) End() error {
	switch {
	case r.Error != "":
		return &sandbox.ExitError{Code: -1, Reason: sandbox.EndFailed, Detail: r.Error}
	case r.OOM:
		return &sandbox.ExitError{Code: max(r.Exit, 128+r.Signal), Reason: sandbox.EndOOM}
	case r.Signal != 0:
		return &sandbox.ExitError{Code: 128 + r.Signal, Reason: sandbox.EndSignaled}
	case r.Exit != 0:
		return &sandbox.ExitError{Code: r.Exit, Reason: sandbox.EndExited}
	}
	return nil
}

// Ports are the vsock ports of a plan's endpoints, by name.
func Ports(pl netproxy.Plan) map[string]int {
	ports := map[string]int{netproxy.ProxyName: PortProxy}
	for i, p := range pl.Expose {
		ports[netproxy.ExposeName(p)] = PortExpose + i
	}
	return ports
}
