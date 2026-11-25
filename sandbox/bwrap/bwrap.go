// Package bwrap isolates a process with bubblewrap on Linux: storage and exec
// confined, the network open, none, or an allowlist through a proxy on the
// host (sandbox/netproxy), and, through a transient systemd user scope,
// memory and task limits. Importing it registers the runtime.
package bwrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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
		sandbox.AxisNetwork:     {sandbox.NetworkOpen, sandbox.NetworkNone, sandbox.NetworkAllowlist},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited, sandbox.ResourcesLimited},
	}}
}

// Check runs bubblewrap once with the namespaces Start uses.
func (Runtime) Check(ctx context.Context) error {
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

// CheckLimited reports whether a process can be started under limits: a
// systemd user session that delegates the memory and pids controllers. It
// starts a scope with both set and looks for the files that prove them.
func CheckLimited(ctx context.Context) error {
	bin, err := lookSystemdRun()
	if err != nil {
		return fmt.Errorf("systemd-run is not installed: resource limits need a systemd user session")
	}
	probe := `p=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup); test -r "$p/memory.max" && test -r "$p/pids.max"`
	out, err := exec.CommandContext(ctx, bin, "--user", "--scope", "--quiet", "--collect",
		"-p", "MemoryMax=64M", "-p", "TasksMax=64", "--", "sh", "-c", probe).CombinedOutput()
	if err != nil {
		return fmt.Errorf("no systemd user scope with memory and pids limits here (%v: %s): needs a systemd user session whose cgroup delegates memory and pids", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	bin, err := lookPath()
	if err != nil {
		return nil, fmt.Errorf("%w: bubblewrap (bwrap) is not installed", sandbox.ErrUnavailable)
	}
	var extra []string
	var proxy *netproxy.Proxy
	if p.Network.Level == sandbox.NetworkAllowlist {
		// No network inside; the bridge (this binary) offers the host's
		// proxy on loopback, through its socket.
		if proxy, err = netproxy.Listen(p.Network.Allow); err != nil {
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
		if cmd, err = netproxy.Wrap(cmd, filepath.Join(proxyDir, netproxy.SocketName)); err != nil {
			_ = proxy.Close()
			return nil, err
		}
		extra = []string{"--ro-bind", proxy.Dir(), proxyDir, "--ro-bind", cmd.Args[0], cmd.Args[0]}
	}
	proc, err := start(ctx, bin, cmd, p, extra)
	if err != nil {
		if proxy != nil {
			_ = proxy.Close()
		}
		return nil, err
	}
	if proxy != nil {
		return netproxy.Serve(proxy, proc), nil
	}
	return proc, nil
}

// proxyDir is where the sandbox sees the proxy's socket directory.
const proxyDir = "/run/tb-sandbox-net"

// start runs cmd in bubblewrap, with extra arguments after the mounts, inside
// a limited systemd scope when p asks for limits.
func start(ctx context.Context, bin string, cmd sandbox.Cmd, p sandbox.Policy, extra []string) (sandbox.Proc, error) {
	args, err := buildArgs(p, cmd.Dir, extra)
	if err != nil {
		return nil, err
	}
	prefix := []string{bin}
	if p.Resources.Level == sandbox.ResourcesLimited {
		// Never without the limits: if the scope cannot be had, do not start.
		if err := CheckLimited(ctx); err != nil {
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
		run, err := lookSystemdRun()
		if err != nil {
			return nil, fmt.Errorf("%w: systemd-run is not installed", sandbox.ErrUnavailable)
		}
		// systemd-run reaches the user manager through the host's session
		// variables and execs bwrap with its own environment. The process
		// must get only cmd.Env, so bwrap puts those variables back as cmd.Env
		// had them (never the environment on a command line: argv is public).
		prefix = append(scopeArgs(run, p.Resources), bin)
		var restore []string
		for _, k := range []string{"XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
			if v, ok := envLookup(cmd.Env, k); ok {
				restore = append(restore, "--setenv", k, v)
			} else {
				restore = append(restore, "--unsetenv", k)
			}
			if v, ok := os.LookupEnv(k); ok {
				cmd.Env = append(slices.DeleteFunc(slices.Clone(cmd.Env), func(kv string) bool { return strings.HasPrefix(kv, k+"=") }), k+"="+v)
			}
		}
		args = append(restore, args...)
	}
	cmd.Args = append(append(prefix, args...), cmd.Args...)
	// bwrap changes into cmd.Dir inside; outside it starts where we are.
	cmd.Dir = ""
	return sandbox.ExecCommand(ctx, cmd)
}

func envLookup(env []string, name string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v, true
		}
	}
	return "", false
}

// scopeArgs is the command that runs what follows in a transient systemd user
// scope with r's limits. A zero limit is left out. Swap is off with the memory
// limit, so the process cannot outgrow it by swapping.
func scopeArgs(run string, r sandbox.Resources) []string {
	args := []string{run, "--user", "--scope", "--quiet", "--collect"}
	if r.MemoryBytes > 0 {
		args = append(args, "-p", "MemoryMax="+strconv.FormatInt(r.MemoryBytes, 10), "-p", "MemorySwapMax=0")
	}
	if r.Tasks > 0 {
		args = append(args, "-p", "TasksMax="+strconv.Itoa(r.Tasks))
	}
	return append(args, "--")
}

// Args are the bubblewrap arguments for p, up to and including "--": an empty
// root with the host's system directories read-only, a private /proc, /dev and
// /tmp, an empty Home, the mounts, and the process in its own user, PID, IPC
// and UTS namespaces with no capabilities, ended with its parent.
func Args(p sandbox.Policy, dir string) ([]string, error) { return buildArgs(p, dir, nil) }

// buildArgs is Args with extra arguments after the mounts, so they land on
// top of them.
func buildArgs(p sandbox.Policy, dir string, extra []string) ([]string, error) {
	home := p.Storage.Home
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("%w: sandbox home %q is not an absolute path", sandbox.ErrUnavailable, home)
	}
	args := []string{
		"--unshare-user", "--disable-userns", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--cap-drop", "ALL", "--new-session", "--die-with-parent",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	if p.Network.Level != sandbox.NetworkOpen {
		args = append(args, "--unshare-net")
	}
	args = append(args, systemMounts()...)
	args = append(args, "--tmpfs", home)
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
		args = append(args, flag, m.Path, m.Path)
	}
	args = append(args, extra...)
	if dir != "" {
		args = append(args, "--chdir", dir)
	}
	return append(args, "--"), nil
}

// sortMounts orders mounts so a path comes after every mount that contains it.
func sortMounts(ms []sandbox.Mount) []sandbox.Mount {
	out := slices.Clone(ms)
	slices.SortStableFunc(out, func(a, b sandbox.Mount) int { return depth(a.Path) - depth(b.Path) })
	return out
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

// systemMounts are the host's system directories, read-only: /usr and /etc,
// the top-level links or directories a merged /usr leaves (/bin, /lib, …),
// /opt when present, and the files /etc links to outside these (on systemd
// hosts /etc/resolv.conf points into /run, which is not mounted).
func systemMounts() []string {
	args := []string{"--ro-bind", "/usr", "/usr", "--ro-bind", "/etc", "/etc"}
	for _, d := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32"} {
		fi, err := os.Lstat(d)
		switch {
		case err != nil:
		case fi.Mode()&os.ModeSymlink != 0:
			if target, err := os.Readlink(d); err == nil {
				args = append(args, "--symlink", target, d)
			}
		case fi.IsDir():
			args = append(args, "--ro-bind", d, d)
		}
	}
	if fi, err := os.Stat("/opt"); err == nil && fi.IsDir() {
		args = append(args, "--ro-bind", "/opt", "/opt")
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
		args = append(args, "--ro-bind-try", target, target)
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
