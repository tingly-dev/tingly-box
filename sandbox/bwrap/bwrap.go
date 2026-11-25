// Package bwrap isolates a process with bubblewrap on Linux: storage and exec
// confined, the network open or none. Importing it registers the runtime.
package bwrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// Name is the runtime's name in the registry and in settings.
const Name = "bwrap"

func init() { sandbox.Register(Runtime{}) }

// Runtime is the bubblewrap runtime.
type Runtime struct{}

// lookPath finds bubblewrap; a variable for tests.
var lookPath = func() (string, error) { return exec.LookPath("bwrap") }

func (Runtime) Info() sandbox.Info {
	return sandbox.Info{Name: Name, Isolation: sandbox.IsolationProcess, Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage:     {sandbox.StorageConfined},
		sandbox.AxisNetwork:     {sandbox.NetworkOpen, sandbox.NetworkNone},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited},
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

func (Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	args, err := Args(p, cmd.Dir)
	if err != nil {
		return nil, err
	}
	bin, err := lookPath()
	if err != nil {
		return nil, fmt.Errorf("%w: bubblewrap (bwrap) is not installed", sandbox.ErrUnavailable)
	}
	cmd.Args = append(append([]string{bin}, args...), cmd.Args...)
	// bwrap changes into cmd.Dir inside; outside it starts where we are.
	cmd.Dir = ""
	return sandbox.ExecCommand(ctx, cmd)
}

// Args are the bubblewrap arguments for p, up to and including "--": an empty
// root with the host's system directories read-only, a private /proc, /dev and
// /tmp, an empty Home, the mounts, and the process in its own user, PID, IPC
// and UTS namespaces with no capabilities, ended with its parent.
func Args(p sandbox.Policy, dir string) ([]string, error) {
	home := p.Storage.Home
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("%w: sandbox home %q is not an absolute path", sandbox.ErrUnavailable, home)
	}
	args := []string{
		"--unshare-user", "--disable-userns", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--cap-drop", "ALL", "--new-session", "--die-with-parent",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	if p.Network.Level == sandbox.NetworkNone {
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
