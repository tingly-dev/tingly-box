//go:build darwin

// Package macvm isolates a process in a Linux virtual machine on macOS, made
// with Apple's Virtualization.framework (vz_darwin.go). Its root is a
// read-only guest image (image.go); the policy's mounts are virtio-fs shares
// whose read-only is enforced by the host; the internet level is a filtered
// user-space network on the host side of the VM's card (sandbox/usernet);
// the guest's init and its protocol over vsock are microvm's
// (sandbox/vmguest). Importing it registers the runtime, which on any other
// system, or without cgo, is listed and says why it is unavailable.
// The design: .design/sandbox-macvm.md. Not run on a real Mac yet.
package macvm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
	"github.com/tingly-dev/tingly-box/sandbox/netproxy"
	"github.com/tingly-dev/tingly-box/sandbox/usernet"
	"github.com/tingly-dev/tingly-box/sandbox/vmguest"
)

// Name is the runtime's name in the registry and in settings.
const Name = "macvm"

func init() { sandbox.Register(Runtime{}) }

// Runtime is the macOS VM runtime.
type Runtime struct{}

// goos is the system this runs on; a variable for tests.
var goos = runtime.GOOS

const (
	shareTag = "tb"
	guestMAC = "52:54:00:12:34:56"
	// The guest boots from the image's root disk; vsock, virtio-fs and the
	// console are built into the image's kernel.
	cmdline = "console=hvc0 quiet panic=-1 root=/dev/vda ro rootfstype=erofs init=/sbin/tb-guest " + vmguest.Env + "=1"
)

func (Runtime) Info() sandbox.Info {
	return sandbox.Info{Name: Name, Isolation: sandbox.IsolationVM, System: "linux", Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage:     {sandbox.StorageConfined},
		sandbox.AxisNetwork:     {sandbox.NetworkNone, sandbox.NetworkAllowlist, sandbox.NetworkInternet},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited, sandbox.ResourcesLimited},
	}}
}

// available reports why VMs cannot be made here, or nil.
func available() error {
	switch {
	case goos != "darwin":
		return fmt.Errorf("the macOS VM runtime runs on macOS only (this is %s), where it uses Apple's Virtualization.framework; on Linux use microvm", goos)
	case backend == nil:
		return errors.New("this build has no cgo, which the macOS VM runtime needs for Virtualization.framework: build with CGO_ENABLED=1")
	}
	return backend.Check()
}

// Check looks for the hypervisor and the guest image.
func (Runtime) Check(ctx context.Context) error {
	if err := available(); err != nil {
		return err
	}
	dir, err := imageDir()
	if err != nil {
		return err
	}
	_, err = LoadImage(dir)
	return err
}

func (Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	if err := available(); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	dir, err := imageDir()
	if err == nil {
		var img *Image
		if img, err = LoadImage(dir); err == nil {
			return start(ctx, cmd, p, img)
		}
	}
	return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
}

func start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy, img *Image) (sandbox.Proc, error) {
	dir, err := os.MkdirTemp("", "tb-macvm-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	m := &machine{dir: dir, done: make(chan struct{}), stderr: cmd.Stderr}
	if err := m.start(ctx, cmd, p, img); err != nil {
		m.stop()
		return nil, err
	}
	return m, nil
}

// machine is one running VM and what serves it on the host.
type machine struct {
	dir    string
	stderr io.Writer

	vm        vm
	listeners []net.Listener
	net       *netproxy.Host
	inet      *usernet.Network
	stdin     net.Conn
	stdout    net.Conn

	done     chan struct{}
	waitErr  error
	killed   atomic.Bool
	stopOnce sync.Once
}

func (m *machine) start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy, img *Image) error {
	unavailable := func(err error) error { return fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err) }
	shares, mounts, err := planMounts(p.Storage.Mounts, filepath.Join(m.dir, "stage"))
	if err != nil {
		return unavailable(err)
	}
	cur, err := user.Current()
	if err != nil {
		return unavailable(err)
	}

	maxCPUs, maxMemoryMB := backend.Limits()
	cpus, memoryMB := guestSize(p.Resources, maxCPUs, maxMemoryMB)
	cfg := vmConfig{Kernel: img.Kernel(), Rootfs: img.Rootfs(), Cmdline: cmdline, Console: filepath.Join(m.dir, "console.log"),
		CPUs: cpus, MemoryMB: memoryMB, ShareTag: shareTag, Shares: shares}

	s := vmguest.Spec{Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir, UID: os.Getuid(), GID: os.Getgid(),
		Root: true, Fresh: []string{p.Storage.Home, "/tmp", "/run"}, Tag: shareTag, Mounts: mounts,
		User: cur.Username, Foreign: true}
	if p.Resources.Level == sandbox.ResourcesLimited {
		s.Limits = &p.Resources
	}

	if p.Network.Level == sandbox.NetworkInternet {
		grants, byName := netfilter.Grants(p.Network.Allow)
		if len(byName) > 0 {
			return unavailable(fmt.Errorf("the macOS VM runtime grants private destinations under internet by address only, not %v", byName))
		}
		resolver, err := netfilter.HostResolver()
		if err != nil {
			return unavailable(err)
		}
		host, guest, err := usernet.Pair()
		if err != nil {
			return unavailable(err)
		}
		if m.inet, err = usernet.Serve(host, usernet.Config{Grants: grants, Resolver: resolver}); err != nil {
			_ = host.Close()
			_ = guest.Close()
			return unavailable(err)
		}
		cfg.Card, cfg.MAC = guest, guestMAC
		s.Net = &vmguest.Net{Addr: usernet.Guest.String(), Gateway: usernet.Gateway.String()}
		s.ResolvConf = string(netfilter.ResolvConf())
	}

	if m.vm, err = backend.New(cfg); err != nil {
		return unavailable(err)
	}
	// The ports the guest connects to.
	conns := map[int]chan net.Conn{}
	for _, port := range []int{vmguest.PortControl, vmguest.PortStdin, vmguest.PortStdout, vmguest.PortStderr} {
		ln, err := m.vm.Listen(uint32(port))
		if err != nil {
			return unavailable(err)
		}
		m.listeners = append(m.listeners, ln)
		ch := make(chan net.Conn, 1)
		conns[port] = ch
		go func() {
			if c, err := ln.Accept(); err == nil {
				ch <- c
			}
			_ = ln.Close()
		}()
	}
	pl := netproxy.PlanFor(p.Network)
	if !pl.Empty() {
		ports := vmguest.Ports(pl)
		if m.net, err = netproxy.ListenOn(p.Network, func(name string) (net.Listener, error) { return m.vm.Listen(uint32(ports[name])) }); err != nil {
			return unavailable(err)
		}
		s.Proxy, s.Expose = pl.Proxy, pl.Expose
	}

	if err := m.vm.Start(); err != nil {
		return unavailable(fmt.Errorf("the VM did not start: %v%s", err, m.tail()))
	}
	accept := func(port int) (net.Conn, error) {
		select {
		case c := <-conns[port]:
			return c, nil
		case <-m.vm.Done():
			return nil, unavailable(fmt.Errorf("the VM stopped while booting%s", m.tail()))
		case <-time.After(60 * time.Second):
			return nil, unavailable(fmt.Errorf("the VM did not boot in 60s%s", m.tail()))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctl, err := accept(vmguest.PortControl)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(ctl).Encode(s); err != nil {
		return unavailable(err)
	}
	var streams [3]net.Conn
	for i, port := range []int{vmguest.PortStdin, vmguest.PortStdout, vmguest.PortStderr} {
		if streams[i], err = accept(port); err != nil {
			return err
		}
	}
	m.stdin, m.stdout = streams[0], streams[1]
	go func() {
		w := m.stderr
		if w == nil {
			w = io.Discard
		}
		_, _ = io.Copy(w, streams[2])
	}()
	go m.watch(ctl)
	return nil
}

// watch waits for the guest's result and the VM's end, then cleans up.
func (m *machine) watch(ctl net.Conn) {
	var r vmguest.Result
	line, err := bufio.NewReader(ctl).ReadBytes('\n')
	switch {
	case m.killed.Load():
		m.waitErr = &sandbox.ExitError{Code: 137, Reason: sandbox.EndKilled}
	case err != nil:
		m.waitErr = &sandbox.ExitError{Code: -1, Reason: sandbox.EndFailed, Detail: "the guest ended without a result" + m.tail()}
	case json.Unmarshal(line, &r) != nil:
		m.waitErr = &sandbox.ExitError{Code: -1, Reason: sandbox.EndFailed, Detail: fmt.Sprintf("bad result from the guest: %q", line)}
	default:
		m.waitErr = r.End()
	}
	// The guest powers off after it reports; give it a moment, then stop it.
	select {
	case <-m.vm.Done():
	case <-time.After(5 * time.Second):
	}
	m.stop()
	close(m.done)
}

// stop ends the VM and removes what it left; it is idempotent.
func (m *machine) stop() {
	m.stopOnce.Do(func() {
		if m.vm != nil {
			_ = m.vm.Stop()
		}
		for _, ln := range m.listeners {
			_ = ln.Close()
		}
		if m.net != nil {
			_ = m.net.Close()
		}
		if m.inet != nil {
			_ = m.inet.Close()
		}
		_ = os.RemoveAll(m.dir)
	})
}

func (m *machine) Stdin() io.WriteCloser { return m.stdin }
func (m *machine) Stdout() io.ReadCloser { return m.stdout }
func (m *machine) Done() <-chan struct{} { return m.done }

func (m *machine) Wait() error {
	<-m.done
	return m.waitErr
}

// Kill ends the VM, and with it every process in it.
func (m *machine) Kill() error {
	select {
	case <-m.done:
		return nil
	default:
	}
	if m.vm == nil {
		return nil
	}
	m.killed.Store(true)
	return m.vm.Stop()
}

// tail is the end of the VM's console, for an error message.
func (m *machine) tail() string {
	data, _ := os.ReadFile(filepath.Join(m.dir, "console.log"))
	s := string(data)
	if len(s) > 2000 {
		s = s[len(s)-2000:]
	}
	if s == "" {
		return ""
	}
	return "\nconsole:\n" + s
}
