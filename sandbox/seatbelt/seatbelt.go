// Package seatbelt isolates a process with macOS Seatbelt (sandbox-exec and
// an SBPL profile). Apple has deprecated the command, but it is the one
// isolation macOS gives without a virtual machine, so it is offered for what
// it can give (.design/sandbox-macvm.md §10): storage confined, the network
// none or through an allowlist proxy on the host (or open), exec confined as
// far as a profile goes, unlimited resources. It cannot give the internet
// level (a profile cannot say "public addresses, not the host's or the
// local network's") nor limits, and Check says so. Importing it registers
// the runtime. Not run on a real Mac yet: the profile may need tuning.
package seatbelt

import (
	"context"
	"fmt"
	"net"
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
const Name = "seatbelt"

func init() { sandbox.Register(Runtime{}) }

// Runtime is the Seatbelt runtime.
type Runtime struct{}

// goos is the system this runs on; a variable for tests.
var goos = runtime.GOOS

// sandboxExec is the command; a variable for tests.
var sandboxExec = "/usr/bin/sandbox-exec"

func (Runtime) Info() sandbox.Info {
	return sandbox.Info{Name: Name, Isolation: sandbox.IsolationProcess, Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage:     {sandbox.StorageConfined},
		sandbox.AxisNetwork:     {sandbox.NetworkNone, sandbox.NetworkAllowlist, sandbox.NetworkOpen},
		sandbox.AxisExec:        {sandbox.ExecConfined},
		sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
		sandbox.AxisResources:   {sandbox.ResourcesUnlimited},
	}}
}

// Check runs sandbox-exec once.
func (Runtime) Check(ctx context.Context) error {
	if goos != "darwin" {
		return fmt.Errorf("Seatbelt runs on macOS only (this is %s)", goos)
	}
	out, err := exec.CommandContext(ctx, sandboxExec, "-p", "(version 1)(deny default)(allow process-exec)(allow file-read*)", "/usr/bin/true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox-exec does not run here (%v: %s): it cannot start inside another sandbox", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r Runtime) Start(ctx context.Context, cmd sandbox.Cmd, p sandbox.Policy) (sandbox.Proc, error) {
	if goos != "darwin" {
		return nil, fmt.Errorf("%w: Seatbelt runs on macOS only (this is %s)", sandbox.ErrUnavailable, goos)
	}
	fresh, err := os.MkdirTemp("", "tb-seatbelt-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	// The profile matches real paths.
	if real, err := filepath.EvalSymlinks(fresh); err == nil {
		fresh = real
	}
	cleanup := func() { _ = os.RemoveAll(fresh) }
	home, tmp := filepath.Join(fresh, "home"), filepath.Join(fresh, "tmp")
	for _, d := range []string{home, tmp} {
		if err := os.Mkdir(d, 0o700); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
	}

	pl := plan{policy: p, fresh: fresh}
	var host *netproxy.Host
	if p.Network.Level == sandbox.NetworkAllowlist {
		// The proxy listens on the host's loopback, where the profile lets
		// the sandbox connect to it and to the exposed ports only.
		n := p.Network
		n.Expose = nil
		if host, err = netproxy.ListenOn(n, func(string) (net.Listener, error) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err == nil {
				pl.proxyPort = ln.Addr().(*net.TCPAddr).Port
			}
			return ln, err
		}); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
		}
	}
	sbpl, err := profile(pl)
	if err != nil {
		cleanup()
		if host != nil {
			_ = host.Close()
		}
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}

	cmd.Env = environment(cmd.Env, home, tmp, pl.proxyPort)
	cmd.Args = append([]string{sandboxExec, "-p", sbpl, "--"}, cmd.Args...)
	proc, err := sandbox.ExecCommand(ctx, cmd)
	if err != nil {
		cleanup()
		if host != nil {
			_ = host.Close()
		}
		return nil, err
	}
	go func() {
		<-proc.Done()
		cleanup()
		if host != nil {
			_ = host.Close()
		}
	}()
	return proc, nil
}

// environment is env with HOME and TMPDIR in the sandbox's own directory
// (the real home is not readable) and the proxy named when there is one.
func environment(env []string, home, tmp string, proxyPort int) []string {
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return name == "HOME" || name == "TMPDIR"
	})
	out = append(out, "HOME="+home, "TMPDIR="+tmp)
	if proxyPort != 0 {
		out = append(out, proxyEnv(proxyPort)...)
	}
	return out
}
