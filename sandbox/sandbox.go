// Package sandbox is the one place that knows how an agent process is
// isolated. It defines the behaviour model (Policy: what is isolated, axis by
// axis) and the interface every isolation runtime implements (Runtime), and
// keeps the registry of runtimes a build ships. Callers say what they want in
// a Policy and name a runtime; they never know how the runtime does it. A new
// runtime is a package that implements Runtime, registers itself and passes
// the conformance suite (sandbox/conformance); nothing else changes.
//
// The model and the reasoning behind it: projects/.design/thread-runtime.md.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Axis is one dimension of isolation. Each is configured on its own, and a
// runtime declares which levels it can give on each.
type Axis string

const (
	// AxisStorage is which files the process sees and may write.
	AxisStorage Axis = "storage"
	// AxisNetwork is where the process may connect.
	AxisNetwork Axis = "network"
	// AxisExec is what the process may run and see of other processes:
	// privileges, namespaces, devices, signals.
	AxisExec Axis = "exec"
	// AxisCredentials is which secrets reach the process: environment
	// variables here; files and injected tokens through the other axes.
	AxisCredentials Axis = "credentials"
	// AxisResources is how much CPU, memory and how many processes it may use.
	AxisResources Axis = "resources"
)

// Axes are all axes, in the order they are shown.
var Axes = []Axis{AxisStorage, AxisNetwork, AxisExec, AxisCredentials, AxisResources}

// Level is how strictly one axis is isolated. Each axis has its own levels.
type Level string

const (
	// Storage: the host's whole filesystem, or only the declared mounts.
	StorageHost     Level = "host"
	StorageConfined Level = "confined"
	// Network: anything the host reaches, or nothing at all (not even the
	// host's own services).
	NetworkOpen Level = "open"
	NetworkNone Level = "none"
	// Exec: as the host user, or in its own namespaces with no privileges,
	// unable to see, signal or trace other processes, killed as a whole.
	ExecHost     Level = "host"
	ExecConfined Level = "confined"
	// Credentials: the caller's whole environment, or only the variables the
	// policy names.
	CredentialsInherit  Level = "inherit"
	CredentialsExplicit Level = "explicit"
	// Resources: no limits, or the limits the policy sets.
	ResourcesUnlimited Level = "unlimited"
	ResourcesLimited   Level = "limited"
)

// Policy is what a caller wants isolated, axis by axis. It says what, never
// how: the runtime picks the mechanism. A runtime that cannot give every axis
// the level asked for refuses to start (Validate); it never gives less.
type Policy struct {
	Storage     Storage     `json:"storage"`
	Network     Network     `json:"network"`
	Exec        Exec        `json:"exec"`
	Credentials Credentials `json:"credentials"`
	Resources   Resources   `json:"resources"`
}

// Storage is the storage axis. Confined, the process sees the runtime's
// system directories read-only, a private /tmp, an empty Home, and Mounts;
// nothing else of the host.
type Storage struct {
	Level Level `json:"level"`
	// Mounts appear inside at the same path as on the host, so paths in
	// arguments and files mean the same on both sides. A mount under another
	// one lands on top of it (a read-only file can cover part of a writable
	// directory), whatever their order here.
	Mounts []Mount `json:"mounts,omitempty"`
	// Home is HOME inside: an empty directory, with any Mounts under it on top.
	Home string `json:"home,omitempty"`
}

// Mount is a host path the process sees.
type Mount struct {
	Path     string `json:"path"`
	Writable bool   `json:"writable,omitempty"`
	// Optional skips a path that does not exist instead of failing the start.
	Optional bool `json:"optional,omitempty"`
}

// Network is the network axis.
type Network struct {
	Level Level `json:"level"`
}

// Exec is the execution axis.
type Exec struct {
	Level Level `json:"level"`
}

// Credentials is the credentials axis. Explicit, the process gets only the
// variables of Cmd.Env that Env names (exact names, or prefixes ending in
// "*"), and HOME (Storage.Home when set).
type Credentials struct {
	Level Level    `json:"level"`
	Env   []string `json:"env,omitempty"`
}

// Resources is the resources axis. Zero leaves that resource unlimited.
type Resources struct {
	Level       Level `json:"level"`
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	Tasks       int   `json:"tasks,omitempty"`
}

// Level returns the level p asks for on axis a.
func (p Policy) Level(a Axis) Level {
	switch a {
	case AxisStorage:
		return p.Storage.Level
	case AxisNetwork:
		return p.Network.Level
	case AxisExec:
		return p.Exec.Level
	case AxisCredentials:
		return p.Credentials.Level
	case AxisResources:
		return p.Resources.Level
	}
	return ""
}

// Levels are p's levels by axis.
func (p Policy) Levels() map[Axis]Level {
	out := make(map[Axis]Level, len(Axes))
	for _, a := range Axes {
		out[a] = p.Level(a)
	}
	return out
}

// Cmd is the process to start.
type Cmd struct {
	Args []string
	// Env is the process environment, complete: nothing is inherited.
	Env []string
	// Dir is the working directory, a path the process sees.
	Dir string
	// Stderr receives the process's stderr; nil discards it.
	Stderr io.Writer
}

// Proc is a started process. It has the method set of agentboot's
// process.Handle, so it is one.
//
//   - Stdin and Stdout are valid until Wait returns.
//   - Done is closed after the process has exited.
//   - Kill ends every process in the sandbox; it is idempotent.
type Proc interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Wait() error
	Kill() error
	Done() <-chan struct{}
}

// Isolation is how strong a runtime's boundary is, for people to compare.
type Isolation string

const (
	IsolationNone       Isolation = "none"       // the host itself
	IsolationProcess    Isolation = "process"    // namespaces, shared kernel
	IsolationUserKernel Isolation = "userkernel" // a kernel in user space (gVisor)
	IsolationVM         Isolation = "vm"         // its own kernel (microVM)
	IsolationRemote     Isolation = "remote"     // another machine
)

// Info describes a runtime.
type Info struct {
	Name      string    `json:"name"`
	Isolation Isolation `json:"isolation"`
	// Supports lists, per axis, the levels the runtime can give.
	Supports map[Axis][]Level `json:"supports"`
}

// Runtime is one way of isolating a process. It gives each axis the level
// the policy asks for and nothing less, and is proved by sandbox/conformance.
type Runtime interface {
	Info() Info
	// Check reports whether the runtime can run here; the error says what is
	// missing and how to fix it.
	Check(ctx context.Context) error
	// Start starts cmd under policy. Start validates nothing: call
	// sandbox.Start, which does, and applies what is the same everywhere.
	Start(ctx context.Context, cmd Cmd, policy Policy) (Proc, error)
}

// ErrUnavailable wraps every reason a process could not be confined as asked:
// the caller must not start it some other way.
var ErrUnavailable = errors.New("sandbox unavailable")

// Validate reports whether rt can give every axis the level p asks for.
func Validate(info Info, p Policy) error {
	var missing []string
	for _, a := range Axes {
		if l := p.Level(a); !slices.Contains(info.Supports[a], l) {
			missing = append(missing, fmt.Sprintf("%s=%s", a, orUnset(l)))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: runtime %s cannot give %s", ErrUnavailable, info.Name, strings.Join(missing, ", "))
	}
	return nil
}

func orUnset(l Level) Level {
	if l == "" {
		return "(unset)"
	}
	return l
}

// Start starts cmd in runtime name under p: it checks the runtime is here and
// can give every axis its level, applies what is the same for every runtime
// (the credentials axis, and HOME from Storage.Home), then hands over to the
// runtime.
func Start(ctx context.Context, name string, cmd Cmd, p Policy) (Proc, error) {
	rt, ok := Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: no runtime %q in this build", ErrUnavailable, name)
	}
	if err := Validate(rt.Info(), p); err != nil {
		return nil, err
	}
	if err := rt.Check(ctx); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnavailable, name, err)
	}
	if len(cmd.Args) == 0 {
		return nil, errors.New("sandbox: empty command")
	}
	if p.Credentials.Level == CredentialsExplicit {
		cmd.Env = FilterEnv(cmd.Env, p.Credentials.Env, p.Storage.Home)
	} else if p.Storage.Home != "" {
		cmd.Env = FilterEnv(cmd.Env, []string{"*"}, p.Storage.Home)
	}
	return rt.Start(ctx, cmd, p)
}

// FilterEnv keeps the variables keep names (exact, or a prefix ending in "*"),
// and sets HOME to home when it is not empty.
func FilterEnv(env, keep []string, home string) []string {
	var out []string
	if home != "" {
		out = append(out, "HOME="+home)
	}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if home != "" && name == "HOME" {
			continue
		}
		if keeps(keep, name) {
			out = append(out, kv)
		}
	}
	return out
}

func keeps(keep []string, name string) bool {
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

var (
	mu       sync.RWMutex
	runtimes = map[string]Runtime{}
)

// Register adds a runtime under its Info().Name. A runtime package calls it
// from init; a build ships the runtimes it imports.
func Register(rt Runtime) {
	mu.Lock()
	defer mu.Unlock()
	name := rt.Info().Name
	if _, dup := runtimes[name]; dup {
		panic("sandbox: runtime " + name + " registered twice")
	}
	runtimes[name] = rt
}

// Get returns the runtime registered under name.
func Get(name string) (Runtime, bool) {
	mu.RLock()
	defer mu.RUnlock()
	rt, ok := runtimes[name]
	return rt, ok
}

// List returns the registered runtimes, by name.
func List() []Runtime {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Runtime, 0, len(runtimes))
	for _, rt := range runtimes {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info().Name < out[j].Info().Name })
	return out
}
