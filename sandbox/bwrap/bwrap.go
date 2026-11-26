// Package bwrap isolates a process with bubblewrap on Linux: storage and exec
// confined; the network none, an allowlist through a proxy on the host, the
// public internet through pasta and a packet filter, or the host's own, with
// host ports exposed (sandbox/netproxy); and, through a transient systemd
// user scope, memory, task and CPU limits and the oom end. Importing it
// registers the runtime.
package bwrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netproxy"
)

// Name is the runtime's name in the registry and in settings.
const Name = "bwrap"

func init() { sandbox.Register(Runtime{}) }

// Runtime is the bubblewrap runtime.
type Runtime struct{}

// lookPath finds bubblewrap; a variable for tests.
var lookPath = func() (string, error) { return exec.LookPath("bwrap") }

// lookSystemdRun finds systemd-run, which puts a limited process in a scope; a
// variable for tests.
var lookSystemdRun = func() (string, error) { return exec.LookPath("systemd-run") }

func (Runtime) Info() sandbox.Info {
	return sandbox.Info{Name: Name, Isolation: sandbox.IsolationProcess, Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage:     {sandbox.StorageConfined},
		sandbox.AxisNetwork:     {sandbox.NetworkOpen, sandbox.NetworkNone, sandbox.NetworkAllowlist, sandbox.NetworkInternet},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited, sandbox.ResourcesLimited},
	}}
}

// Check runs bubblewrap once with the namespaces Start uses.
// goos is the system this runs on; a variable for tests.
var goos = runtime.GOOS

func (Runtime) Check(ctx context.Context) error {
	if goos != "linux" {
		return fmt.Errorf("bubblewrap runs on Linux only (this is %s): it is made of Linux namespaces", goos)
	}
	bin, err := lookPath()
	if err != nil {
		return fmt.Errorf("bubblewrap (bwrap) is not installed: install the bubblewrap package")
	}
	out, err := exec.CommandContext(ctx, bin, "--ro-bind", "/", "/", "--unshare-user", "--disable-userns", "--unshare-pid", "true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("bubblewrap cannot create namespaces here (%v: %s): unprivileged user namespaces may be disabled", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	if goos != "linux" {
		return nil, fmt.Errorf("%w: bubblewrap runs on Linux only (this is %s)", sandbox.ErrUnavailable, goos)
	}
	bin, err := lookPath()
	if err != nil {
		return nil, fmt.Errorf("%w: bubblewrap (bwrap) is not installed", sandbox.ErrUnavailable)
	}
	var extra, via []string
	var closers []io.Closer
	closeAll := func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}
	if p.Network.Level == sandbox.NetworkInternet {
		in, err := newInternet(ctx, p.Network)
		if err != nil {
			return nil, err
		}
		closers = append(closers, in)
		via, extra = in.via, in.extra
	}
	if pl := netproxy.PlanFor(p.Network); !pl.Empty() {
		// The sandbox's own network reaches the host only through the
		// bridge (this binary), which offers the host's endpoints on the
		// sandbox's loopback through their sockets.
		host, err := netproxy.ListenDir(p.Network)
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
		closers = append(closers, host)
		if cmd, err = netproxy.Wrap(cmd, netDir, pl); err != nil {
			closeAll()
			return nil, err
		}
		extra = append(extra, "--ro-bind", host.Dir(), netDir, "--ro-bind", cmd.Args[0], cmd.Args[0])
	}
	proc, err := start(ctx, bin, cmd, p, via, extra)
	if err != nil {
		closeAll()
		return nil, err
	}
	if len(closers) > 0 {
		go func() {
			<-proc.Done()
			closeAll()
		}()
	}
	return proc, nil
}

// netDir is where the sandbox sees the host's endpoint sockets.
const netDir = "/run/tb-sandbox-net"

// start runs cmd in bubblewrap, with extra arguments after the mounts, and
// via (the command that runs bubblewrap, if any) before it.
func start(ctx context.Context, bin string, cmd sandbox.Cmd, p sandbox.Policy, via, extra []string) (sandbox.Proc, error) {
	args, err := buildArgs(p, cmd.Dir, extra)
	if err != nil {
		return nil, err
	}
	// In a systemd scope: always when the host has one, for the accounting
	// (the end reason oom); limits require it.
	sc, err := newScope(ctx, p.Resources)
	if err != nil {
		return nil, err
	}
	var prefix []string
	if sc != nil {
		var restore []string
		prefix, restore, cmd.Env = sc.wrap(cmd.Env)
		args = append(restore, args...)
	}
	prefix = append(append(prefix, via...), bin)
	cmd.Args = append(append(prefix, args...), cmd.Args...)
	// bwrap changes into cmd.Dir inside; outside it starts where we are.
	cmd.Dir = ""
	proc, err := sandbox.ExecCommand(ctx, cmd)
	if err != nil || sc == nil {
		return proc, err
	}
	return sc.watch(proc), nil
}

// Args are the bubblewrap arguments for p, up to and including "--": an empty
// root with the host's system directories read-only, a private /proc, /dev and
// /tmp, an empty Home, the mounts, and the process in its own user, PID, IPC
// and UTS namespaces with no capabilities, ended with its parent.
func Args(p sandbox.Policy, dir string) ([]string, error) { return buildArgs(p, dir, nil) }

// buildArgs is Args with extra arguments after the mounts, so they land on
// top of them.
func buildArgs(p sandbox.Policy, dir string, extra []string) ([]string, error) {
	args := []string{
		"--unshare-user", "--disable-userns", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--cap-drop", "ALL", "--new-session", "--die-with-parent",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	// Internet's network is pasta's, which bubblewrap runs in; open is
	// the host's.
	if p.Network.Level == sandbox.NetworkNone || p.Network.Level == sandbox.NetworkAllowlist {
		args = append(args, "--unshare-net")
	}
	storage, err := StorageArgs(p, "")
	if err != nil {
		return nil, err
	}
	args = append(args, storage...)
	args = append(args, extra...)
	if dir != "" {
		args = append(args, "--chdir", dir)
	}
	return append(args, "--"), nil
}

// StorageArgs are the bubblewrap arguments that build p's confined storage
// under root ("" for the root itself): the host's system directories
// read-only, an empty Home, and the mounts, each at its own path under root.
// A runtime that serves the storage from the host (a microVM's shared
// directory) builds it with these.
func StorageArgs(p sandbox.Policy, root string) ([]string, error) {
	home := p.Storage.Home
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("%w: sandbox home %q is not an absolute path", sandbox.ErrUnavailable, home)
	}
	args := systemMounts(root)
	args = append(args, "--tmpfs", root+home)
	for _, m := range sortMounts(p.Storage.Mounts) {
		if !filepath.IsAbs(m.Path) {
			return nil, fmt.Errorf("%w: mount %q is not an absolute path", sandbox.ErrUnavailable, m.Path)
		}
		flag := "--ro-bind"
		if m.Writable {
			flag = "--bind"
		}
		if m.Optional {
			flag += "-try"
		}
		args = append(args, flag, m.Path, root+m.Path)
	}
	return args, nil
}

// sortMounts orders mounts so a path comes after every mount that contains it.
func sortMounts(ms []sandbox.Mount) []sandbox.Mount {
	out := slices.Clone(ms)
	slices.SortStableFunc(out, func(a, b sandbox.Mount) int { return depth(a.Path) - depth(b.Path) })
	return out
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

// SystemArgs are the bubblewrap arguments that put the host's system
// directories, read-only, under root (see StorageArgs).
func SystemArgs(root string) []string { return systemMounts(root) }

// systemMounts are, under root, the host's system directories read-only: /usr
// and /etc, the top-level links or directories a merged /usr leaves (/bin,
// /lib, …), /opt when present, and the files /etc links to outside these (on
// systemd hosts /etc/resolv.conf points into /run, which is not mounted).
func systemMounts(root string) []string {
	args := []string{"--ro-bind", "/usr", root + "/usr", "--ro-bind", "/etc", root + "/etc"}
	for _, d := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32"} {
		fi, err := os.Lstat(d)
		switch {
		case err != nil:
		case fi.Mode()&os.ModeSymlink != 0:
			if target, err := os.Readlink(d); err == nil {
				args = append(args, "--symlink", target, root+d)
			}
		case fi.IsDir():
			args = append(args, "--ro-bind", d, root+d)
		}
	}
	if fi, err := os.Stat("/opt"); err == nil && fi.IsDir() {
		args = append(args, "--ro-bind", "/opt", root+"/opt")
	}
	entries, _ := os.ReadDir("/etc")
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := filepath.EvalSymlinks(filepath.Join("/etc", e.Name()))
		if err != nil || underAny(target, "/usr", "/etc", "/opt", "/proc", "/dev", "/sys") {
			continue
		}
		// Files only (resolv.conf and the like): a linked directory could
		// carry anything.
		if fi, err := os.Stat(target); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		args = append(args, "--ro-bind-try", target, root+target)
	}
	return args
}

func underAny(p string, roots ...string) bool {
	for _, r := range roots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}
