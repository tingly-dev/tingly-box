// Package microvm isolates a process in its own virtual machine: a Cloud
// Hypervisor microVM with its own kernel, whose root file system is the
// policy's confined storage served from the host by virtiofsd. Importing it
// registers the runtime.
//
// The storage is built exactly as bwrap builds it (bwrap.StorageArgs), but
// under a directory that virtiofsd, itself confined by bubblewrap, shares as
// the guest's root. Read-only mounts stay read-only on the host side, so the
// guest's kernel cannot change that. The guest's init is this same binary
// (see guest_linux.go); it talks to the host over vsock: a control
// connection for the command and its exit, and one each for stdin, stdout
// and stderr. The guest has no network device: network none is the default,
// and allowlist reaches the host's proxy (sandbox/netproxy) over vsock too.
package microvm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/bwrap"
	"github.com/tingly-dev/tingly-box/sandbox/netproxy"
)

// Name is the runtime's name in the registry and in settings.
const Name = "microvm"

func init() { sandbox.Register(Runtime{}) }

// Runtime is the microVM runtime.
type Runtime struct{}

// The vsock ports the guest connects to, on the host (CID 2).
const (
	portControl = 1024 + iota
	portStdin
	portStdout
	portStderr
	portProxy
	// portExpose + i carries Network.Expose[i].
	portExpose = 1100
)

// guestEnv, on the kernel command line, tells this binary it is a guest's
// init (the kernel hands such words to init as its environment).
const guestEnv = "TB_SANDBOX_GUEST"

// spec is what the host sends the guest on the control connection.
type spec struct {
	Args   []string `json:"args"`
	Env    []string `json:"env"`
	Dir    string   `json:"dir"`
	UID    int      `json:"uid"`
	GID    int      `json:"gid"`
	Proxy  bool     `json:"proxy,omitempty"`
	Expose []int    `json:"expose,omitempty"`
}

// result is what the guest sends back when the command has ended, or could
// not start.
type result struct {
	Exit   int    `json:"exit"`
	Signal int    `json:"signal,omitempty"`
	OOM    bool   `json:"oom,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Guest size. The memory is the guest's RAM, allocated as it is touched.
// Variables for tests.
var (
	guestCPUs     = 2
	guestMemoryMB = 2048
)

// What the runtime needs, found by an environment variable, on PATH, or
// where Kata Containers installs it. Variables for tests.
var (
	findVMM = func() (string, error) {
		return findExecutable("TB_SANDBOX_VMM", "cloud-hypervisor", "/opt/kata/bin/cloud-hypervisor")
	}
	findVirtiofsd = func() (string, error) {
		return findExecutable("TB_SANDBOX_VIRTIOFSD", "virtiofsd", "/usr/libexec/virtiofsd", "/opt/kata/libexec/virtiofsd")
	}
	findKernel = func() (string, error) {
		return findFile("TB_SANDBOX_KERNEL", "/opt/kata/share/kata-containers/vmlinux.container")
	}
	findBwrap = func() (string, error) { return exec.LookPath("bwrap") }
)

func (Runtime) Info() sandbox.Info {
	return sandbox.Info{Name: Name, Isolation: sandbox.IsolationVM, Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage:     {sandbox.StorageConfined},
		sandbox.AxisNetwork:     {sandbox.NetworkNone, sandbox.NetworkAllowlist},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited},
	}}
}

// Check looks for KVM, Cloud Hypervisor, virtiofsd, a guest kernel and
// bubblewrap.
func (Runtime) Check(ctx context.Context) error {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("no access to /dev/kvm (%v): add the user to the kvm group and log in again", err)
	}
	_ = f.Close()
	for _, find := range []struct {
		what string
		fn   func() (string, error)
	}{{"Cloud Hypervisor", findVMM}, {"virtiofsd", findVirtiofsd}, {"a guest kernel", findKernel}, {"bubblewrap", findBwrap}} {
		if _, err := find.fn(); err != nil {
			return fmt.Errorf("%s not found: %v", find.what, err)
		}
	}
	if _, err := executable(); err != nil {
		return err
	}
	return nil
}

func (Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	vmm, err1 := findVMM()
	fsd, err2 := findVirtiofsd()
	kernel, err3 := findKernel()
	bw, err4 := findBwrap()
	exe, err5 := executable()
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	dir, err := os.MkdirTemp(socketRoot(), "tb-vm-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	vm := &machine{dir: dir, done: make(chan struct{}), stderr: cmd.Stderr}
	if err := vm.start(ctx, cmd, p, vmm, fsd, kernel, bw, exe); err != nil {
		vm.stop()
		return nil, err
	}
	return vm, nil
}

// machine is one running microVM and the processes that serve it.
type machine struct {
	dir    string
	stderr io.Writer

	fs, vmm   *exec.Cmd
	listeners []net.Listener
	net       *netproxy.Host
	stdin     net.Conn
	stdout    net.Conn

	done     chan struct{}
	waitErr  error
	killed   atomic.Bool
	stopOnce sync.Once
}

func (m *machine) start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy, vmm, fsd, kernel, bw, exe string) error {
	if strings.ContainsAny(exe, " \t\n") {
		return fmt.Errorf("%w: the guest init %q has a space in its path", sandbox.ErrUnavailable, exe)
	}
	// The guest's root, served by virtiofsd from a bubblewrap view: the
	// policy's storage under /view, and this binary as its init.
	const view = "/view"
	storage, err := bwrap.StorageArgs(p, view)
	if err != nil {
		return err
	}
	args := []string{"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-net",
		"--die-with-parent", "--new-session", "--proc", "/proc", "--dev", "/dev"}
	args = append(args, bwrap.SystemArgs("")...)
	// The guest mounts its own /proc, /sys and /dev; /tmp and /run are
	// empty here, under the mounts that may land in them.
	for _, d := range []string{"/proc", "/sys", "/dev"} {
		args = append(args, "--dir", view+d)
	}
	args = append(args, "--perms", "01777", "--tmpfs", view+"/tmp", "--tmpfs", view+"/run")
	args = append(args, storage...)
	args = append(args, "--ro-bind", exe, view+exe)
	sock := filepath.Join(m.dir, "fs.sock")
	args = append(args, "--bind", m.dir, m.dir, "--ro-bind", fsd, fsd,
		"--", fsd, "--socket-path="+sock, "--shared-dir="+view, "--sandbox=none", "--cache=auto", "--announce-submounts", "--log-level=error")
	m.fs = exec.Command(bw, args...)
	m.fs.Stderr = m.logFile("virtiofsd.log")
	if err := m.fs.Start(); err != nil {
		return fmt.Errorf("%w: virtiofsd: %v", sandbox.ErrUnavailable, err)
	}
	if err := waitForSocket(sock, 5*time.Second); err != nil {
		return fmt.Errorf("%w: virtiofsd did not start: %v%s", sandbox.ErrUnavailable, err, m.tail("virtiofsd.log"))
	}

	// The vsock ports the guest connects to.
	vsock := filepath.Join(m.dir, "vsock.sock")
	conns := map[int]chan net.Conn{}
	for _, port := range []int{portControl, portStdin, portStdout, portStderr} {
		ln, err := net.Listen("unix", vsock+"_"+strconv.Itoa(port))
		if err != nil {
			return fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
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
		ports := vsockPorts(pl)
		if m.net, err = netproxy.Listen(p.Network, func(name string) string { return vsock + "_" + strconv.Itoa(ports[name]) }); err != nil {
			return fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
	}

	cmdline := "console=ttyS0 quiet panic=-1 root=root rootfstype=virtiofs rw init=" + exe + " " + guestEnv + "=1"
	m.vmm = exec.Command(vmm, "--kernel", kernel,
		"--cpus", "boot="+strconv.Itoa(guestCPUs),
		"--memory", "size="+strconv.Itoa(guestMemoryMB)+"M,shared=on",
		"--fs", "tag=root,socket="+sock,
		"--vsock", "cid=3,socket="+vsock,
		"--serial", "file="+filepath.Join(m.dir, "console.log"), "--console", "off",
		"--cmdline", cmdline)
	m.vmm.Stdout = m.logFile("vmm.log")
	m.vmm.Stderr = m.vmm.Stdout
	if err := m.vmm.Start(); err != nil {
		return fmt.Errorf("%w: cloud-hypervisor: %v", sandbox.ErrUnavailable, err)
	}
	vmmExited := make(chan struct{})
	go func() { _ = m.vmm.Wait(); close(vmmExited) }()

	accept := func(port int) (net.Conn, error) {
		select {
		case c := <-conns[port]:
			return c, nil
		case <-vmmExited:
			return nil, fmt.Errorf("%w: the microVM stopped while booting%s", sandbox.ErrUnavailable, m.tail("vmm.log", "console.log"))
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("%w: the microVM did not boot in 30s%s", sandbox.ErrUnavailable, m.tail("console.log"))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctl, err := accept(portControl)
	if err != nil {
		return err
	}
	s := spec{Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir, UID: os.Getuid(), GID: os.Getgid(), Proxy: pl.Proxy, Expose: pl.Expose}
	if err := json.NewEncoder(ctl).Encode(s); err != nil {
		return fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	var streams [3]net.Conn
	for i, port := range []int{portStdin, portStdout, portStderr} {
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
	go m.watch(ctl, vmmExited)
	return nil
}

// watch waits for the guest's result and the VM's end, then cleans up.
func (m *machine) watch(ctl net.Conn, vmmExited <-chan struct{}) {
	var r result
	line, err := bufio.NewReader(ctl).ReadBytes('\n')
	failed := func(detail string) error {
		return &sandbox.ExitError{Code: -1, Reason: sandbox.EndFailed, Detail: detail}
	}
	switch {
	case m.killed.Load():
		m.waitErr = &sandbox.ExitError{Code: 128 + int(syscall.SIGKILL), Reason: sandbox.EndKilled}
	case err != nil:
		m.waitErr = failed("the guest ended without a result" + m.tail("console.log"))
	case json.Unmarshal(line, &r) != nil:
		m.waitErr = failed(fmt.Sprintf("bad result from the guest: %q", line))
	case r.Error != "":
		m.waitErr = failed(r.Error)
	case r.OOM:
		m.waitErr = &sandbox.ExitError{Code: max(r.Exit, 128+r.Signal), Reason: sandbox.EndOOM}
	case r.Signal != 0:
		m.waitErr = &sandbox.ExitError{Code: 128 + r.Signal, Reason: sandbox.EndSignaled}
	case r.Exit != 0:
		m.waitErr = &sandbox.ExitError{Code: r.Exit, Reason: sandbox.EndExited}
	}
	// The guest powers off after it reports; give it a moment, then stop it.
	select {
	case <-vmmExited:
	case <-time.After(5 * time.Second):
	}
	m.stop()
	close(m.done)
}

// vsockPorts are the vsock ports of a plan's endpoints, by name.
func vsockPorts(pl netproxy.Plan) map[string]int {
	ports := map[string]int{netproxy.ProxyName: portProxy}
	for i, p := range pl.Expose {
		ports[netproxy.ExposeName(p)] = portExpose + i
	}
	return ports
}

// stop kills the VM and virtiofsd and removes the sockets; it is idempotent.
func (m *machine) stop() {
	m.stopOnce.Do(func() {
		for _, c := range []*exec.Cmd{m.vmm, m.fs} {
			if c != nil && c.Process != nil {
				_ = c.Process.Kill()
			}
		}
		if m.fs != nil && m.fs.Process != nil {
			_ = m.fs.Wait()
		}
		for _, ln := range m.listeners {
			_ = ln.Close()
		}
		if m.net != nil {
			_ = m.net.Close()
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
	if m.vmm != nil && m.vmm.Process != nil {
		m.killed.Store(true)
		return m.vmm.Process.Kill()
	}
	return nil
}

func (m *machine) logFile(name string) *os.File {
	f, err := os.Create(filepath.Join(m.dir, name))
	if err != nil {
		return nil
	}
	return f
}

// tail is the end of the named logs, for an error message.
func (m *machine) tail(names ...string) string {
	var b strings.Builder
	for _, name := range names {
		data, _ := os.ReadFile(filepath.Join(m.dir, name))
		if s := strings.TrimSpace(string(data)); s != "" {
			if len(s) > 2000 {
				s = s[len(s)-2000:]
			}
			fmt.Fprintf(&b, "\n%s:\n%s", name, s)
		}
	}
	return b.String()
}

func waitForSocket(path string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no socket at %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// socketRoot is where a VM's sockets go: the user's runtime directory, else
// the temporary one, whichever keeps socket paths within the length a unix
// socket allows.
func socketRoot() string {
	for _, d := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"} {
		if d != "" && len(d)+len("/tb-vm-0123456789/vsock.sock_1024") < 100 {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return d
			}
		}
	}
	return os.TempDir()
}

// executable is this binary, the guest's init.
func executable() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return "", fmt.Errorf("the guest init (this binary): %v", err)
	}
	return exe, nil
}

func findExecutable(env, name string, candidates ...string) (string, error) {
	if p := os.Getenv(env); p != "" {
		return p, canExec(p)
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	var errs []error
	for _, p := range candidates {
		err := canExec(p)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return "", fmt.Errorf("install %s, or name it in %s", name, env)
}

func canExec(p string) error {
	if _, err := os.Stat(p); err != nil {
		return err
	}
	if err := accessExec(p); err != nil {
		return fmt.Errorf("%s is not executable by this user (%v): install it where you can run it, or name a copy in the environment", p, err)
	}
	return nil
}

func findFile(env string, candidates ...string) (string, error) {
	if p := os.Getenv(env); p != "" {
		_, err := os.Stat(p)
		return p, err
	}
	for _, p := range candidates {
		if f, err := os.Open(p); err == nil {
			_ = f.Close()
			return p, nil
		}
	}
	return "", fmt.Errorf("name a guest kernel (vmlinux with virtiofs and vsock built in) in %s", env)
}
