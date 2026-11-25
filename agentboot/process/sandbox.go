package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Sandbox confines an agent process to the paths it is given, with bubblewrap
// (see projects/.design/thread-runtime.md, "Linux 第一版"). The process sees an
// empty root holding only the host's system directories (read-only), a private
// /proc, /dev and /tmp, an empty HOME, and Mounts; it runs in its own user, PID,
// IPC and UTS namespaces with no capabilities, and gets only the environment
// variables KeepEnv names. The network is the host's (not isolated yet).
type Sandbox struct {
	// Mounts are bound at the same path inside. A mount under another one
	// lands on top of it, so a read-only file can cover part of a writable
	// directory.
	Mounts []Mount
	// Home is HOME inside: an empty tmpfs, with any Mounts under it on top.
	Home string
	// KeepEnv names the variables of LaunchSpec.Env the process gets: exact
	// names, or prefixes ending in "*". HOME is always Home.
	KeepEnv []string
}

// Mount is a host path bound into the sandbox at the same path.
type Mount struct {
	Path     string
	ReadOnly bool
	// Optional skips a path that does not exist instead of failing the launch.
	Optional bool
}

// ErrNoSandbox is returned when a sandbox is asked for and cannot be made: the
// process is never started without one.
var ErrNoSandbox = errors.New("sandbox unavailable")

// bwrapPath finds bubblewrap; a variable for tests.
var bwrapPath = func() (string, error) { return exec.LookPath("bwrap") }

// wrap returns spec with its command run inside the sandbox.
func (s *Sandbox) wrap(spec LaunchSpec) (LaunchSpec, error) {
	bwrap, err := bwrapPath()
	if err != nil {
		return spec, fmt.Errorf("%w: bubblewrap (bwrap) is not installed: %v", ErrNoSandbox, err)
	}
	if s.Home == "" || !filepath.IsAbs(s.Home) {
		return spec, fmt.Errorf("%w: sandbox home %q is not an absolute path", ErrNoSandbox, s.Home)
	}
	args := []string{bwrap,
		"--unshare-user", "--disable-userns", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--cap-drop", "ALL", "--new-session", "--die-with-parent",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	args = append(args, systemMounts()...)
	args = append(args, "--tmpfs", s.Home)
	for _, m := range sortMounts(s.Mounts) {
		if !filepath.IsAbs(m.Path) {
			return spec, fmt.Errorf("%w: mount %q is not an absolute path", ErrNoSandbox, m.Path)
		}
		flag := "--bind"
		if m.ReadOnly {
			flag = "--ro-bind"
		}
		if m.Optional {
			flag += "-try"
		}
		args = append(args, flag, m.Path, m.Path)
	}
	if spec.WorkDir != "" {
		args = append(args, "--chdir", spec.WorkDir)
	}
	args = append(args, "--")
	out := spec
	out.Command = append(args, spec.Command...)
	out.Env = s.env(spec.Env)
	// bwrap chdirs inside; outside it starts where the caller is.
	out.WorkDir = ""
	out.Sandbox = nil
	return out, nil
}

// env keeps the variables KeepEnv names, from env or, when env is nil (inherit),
// from this process's own environment.
func (s *Sandbox) env(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := []string{"HOME=" + s.Home}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name != "HOME" && keepsEnv(s.KeepEnv, name) {
			out = append(out, kv)
		}
	}
	return out
}

func keepsEnv(keep []string, name string) bool {
	for _, k := range keep {
		if p, ok := strings.CutSuffix(k, "*"); ok {
			if strings.HasPrefix(name, p) {
				return true
			}
		} else if k == name {
			return true
		}
	}
	return false
}

// sortMounts orders mounts so a path comes after every mount that contains it;
// otherwise the caller's order is kept.
func sortMounts(ms []Mount) []Mount {
	out := slices.Clone(ms)
	slices.SortStableFunc(out, func(a, b Mount) int {
		return depth(a.Path) - depth(b.Path)
	})
	return out
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

// systemMounts are the host's system directories, read-only: /usr and /etc,
// the top-level links or directories a merged /usr leaves (/bin, /lib, …),
// /opt when present, and the targets of /etc links that point outside these
// (on systemd hosts /etc/resolv.conf points into /run, which is not mounted).
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
