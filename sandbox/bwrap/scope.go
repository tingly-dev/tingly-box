package bwrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// A sandbox runs in a transient systemd user scope: the scope carries its
// limits (resources: limited), and systemd's own accounting tells why it
// ended (a scope the kernel OOM-killed in ends with result oom-kill, and
// systemd stops all of it). Without a systemd user session a limited sandbox
// does not start; an unlimited one runs without accounting
// (.design/sandbox.md §9).

// lookSystemctl finds systemctl, which reads a scope's result; a variable for
// tests.
var lookSystemctl = func() (string, error) { return exec.LookPath("systemctl") }

// scopeChecked is the systemd-run a CheckLimited succeeded with; a failure
// is checked again.
var (
	scopeMu      sync.Mutex
	scopeChecked string
)

// CheckLimited reports whether a process can be started in a scope with
// limits: a systemd user session that delegates the memory and pids
// controllers. It starts a scope with both set and looks for the files that
// prove them; a success is remembered.
func CheckLimited(ctx context.Context) error {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	bin, err := lookSystemdRun()
	if err != nil {
		return fmt.Errorf("systemd-run is not installed: resource limits need a systemd user session")
	}
	if scopeChecked == bin {
		return nil
	}
	probe := `p=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup); test -r "$p/memory.max" && test -r "$p/pids.max"`
	out, err := exec.CommandContext(ctx, bin, "--user", "--scope", "--quiet", "--collect",
		"-p", "MemoryMax=64M", "-p", "TasksMax=64", "--", "sh", "-c", probe).CombinedOutput()
	if err != nil {
		return fmt.Errorf("no systemd user scope with memory and pids limits here (%v: %s): needs a systemd user session whose cgroup delegates memory and pids", err, strings.TrimSpace(string(out)))
	}
	if _, err := lookSystemctl(); err != nil {
		return errors.New("systemctl is not installed: a scope's end cannot be read")
	}
	scopeChecked = bin
	return nil
}

// scope is one sandbox's systemd scope.
type scope struct {
	run, ctl string
	unit     string
	limits   sandbox.Resources
}

// newScope is the scope a sandbox with r runs in: required when r is
// limited, else used when the host has one; nil when it has none.
func newScope(ctx context.Context, r sandbox.Resources) (*scope, error) {
	if err := CheckLimited(ctx); err != nil {
		if r.Level == sandbox.ResourcesLimited {
			// Never without the limits.
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
		return nil, nil
	}
	run, err1 := lookSystemdRun()
	ctl, err2 := lookSystemctl()
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	sc := &scope{run: run, ctl: ctl, unit: "tb-sandbox-" + hex.EncodeToString(b) + ".scope"}
	if r.Level == sandbox.ResourcesLimited {
		sc.limits = r
	}
	return sc, nil
}

// wrap is the command prefix that runs bin in the scope, the bubblewrap
// arguments that keep the host's session variables out of the sandbox, and
// the environment systemd-run needs. systemd-run reaches the user manager
// through the host's session variables and execs bwrap with its own
// environment; bwrap puts those variables back as env had them (never the
// environment on a command line: argv is public).
func (sc *scope) wrap(bin string, env []string) (prefix, restore, newEnv []string) {
	newEnv = env
	for _, k := range []string{"XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		if v, ok := envLookup(env, k); ok {
			restore = append(restore, "--setenv", k, v)
		} else {
			restore = append(restore, "--unsetenv", k)
		}
		if v, ok := os.LookupEnv(k); ok {
			newEnv = append(slices.DeleteFunc(slices.Clone(newEnv), func(kv string) bool { return strings.HasPrefix(kv, k+"=") }), k+"="+v)
		}
	}
	return append(scopeArgs(sc.run, sc.unit, sc.limits), bin), restore, newEnv
}

func envLookup(env []string, name string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v, true
		}
	}
	return "", false
}

// scopeArgs is the command that runs what follows in the transient systemd
// user scope unit with r's limits. A zero limit is left out. Swap is off with
// the memory limit, so the process cannot outgrow it by swapping. The scope
// is not collected when it fails, so its result can be read (and is reset
// then).
func scopeArgs(run, unit string, r sandbox.Resources) []string {
	args := []string{run, "--user", "--scope", "--quiet", "--unit=" + unit}
	if r.MemoryBytes > 0 {
		args = append(args, "-p", "MemoryMax="+strconv.FormatInt(r.MemoryBytes, 10), "-p", "MemorySwapMax=0")
	}
	if r.Tasks > 0 {
		args = append(args, "-p", "TasksMax="+strconv.Itoa(r.Tasks))
	}
	if r.CPU > 0 {
		// CPUQuota is in percent of one core; at least 1%.
		args = append(args, "-p", "CPUQuota="+strconv.Itoa(max(1, r.CPU/10))+"%")
	}
	return append(args, "--")
}

// watch returns proc with its end read from the scope, and its usage.
func (sc *scope) watch(proc sandbox.Proc) sandbox.Proc {
	return &scoped{Proc: proc, sc: sc}
}

// scoped is a sandbox in a scope.
type scoped struct {
	sandbox.Proc
	sc      *scope
	once    sync.Once
	waitErr error
	cgroup  string // found on the first Usage
	mu      sync.Mutex
}

// Wait is the process's end, as oom when systemd says the kernel OOM-killed
// in the scope.
func (s *scoped) Wait() error {
	err := s.Proc.Wait()
	s.once.Do(func() {
		s.waitErr = err
		out, _ := exec.Command(s.sc.ctl, "--user", "show", "-p", "Result", "--value", s.sc.unit).Output()
		if strings.TrimSpace(string(out)) == "oom-kill" {
			code := 137
			var e *sandbox.ExitError
			if errors.As(err, &e) {
				code = e.Code
			}
			s.waitErr = &sandbox.ExitError{Code: code, Reason: sandbox.EndOOM}
		}
		// A failed scope stays loaded until reset; a successful one is gone.
		_ = exec.Command(s.sc.ctl, "--user", "reset-failed", s.sc.unit).Run()
	})
	return s.waitErr
}

// Usage reads the scope's cgroup.
func (s *scoped) Usage() (sandbox.Stats, error) {
	dir, err := s.cgroupDir()
	if err != nil {
		return sandbox.Stats{}, err
	}
	read := func(name string) int64 {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		n, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		return n
	}
	st := sandbox.Stats{MemoryBytes: read("memory.current"), PeakMemoryBytes: read("memory.peak"), Tasks: int(read("pids.current"))}
	if b, err := os.ReadFile(filepath.Join(dir, "cpu.stat")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "usage_usec "); ok {
				us, _ := strconv.ParseInt(v, 10, 64)
				st.CPU = time.Duration(us) * time.Microsecond
			}
		}
	}
	return st, nil
}

// cgroupDir is the scope's cgroup directory, from systemd.
func (s *scoped) cgroupDir() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cgroup != "" {
		return s.cgroup, nil
	}
	out, err := exec.Command(s.sc.ctl, "--user", "show", "-p", "ControlGroup", "--value", s.sc.unit).Output()
	cg := strings.TrimSpace(string(out))
	if err != nil || cg == "" {
		return "", fmt.Errorf("the scope %s has no cgroup (yet): %v", s.sc.unit, err)
	}
	s.cgroup = filepath.Join("/sys/fs/cgroup", cg)
	return s.cgroup, nil
}
