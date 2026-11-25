// Package conformance is what a sandbox runtime must pass: for every axis and
// level the runtime claims (Info().Supports), a probe inside the sandbox tries
// to break the guarantee and must fail. A runtime's own test is one line:
//
//	func TestConformance(t *testing.T) { conformance.Run(t, bwrap.Runtime{}) }
//
// It runs the runtime through sandbox.Start, as callers do, so the runtime
// must be registered. It is skipped where the runtime cannot run (Check).
package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// Run checks rt against every level it claims.
func Run(t *testing.T, rt sandbox.Runtime) {
	t.Helper()
	info := rt.Info()
	if got, ok := sandbox.Get(info.Name); !ok || got.Info().Name != info.Name {
		t.Fatalf("runtime %q is not registered", info.Name)
	}
	if err := rt.Check(context.Background()); err != nil {
		t.Skipf("runtime %s cannot run here: %v", info.Name, err)
	}
	c := &suite{t: t, info: info}
	t.Run("refuses a level it cannot give", c.refuses)
	if c.supports(sandbox.AxisStorage, sandbox.StorageConfined) {
		t.Run("storage/confined", c.storageConfined)
	}
	if c.supports(sandbox.AxisExec, sandbox.ExecConfined) {
		t.Run("exec/confined", c.execConfined)
		t.Run("exec/confined kill ends everything", c.killEndsEverything)
	}
	if c.supports(sandbox.AxisCredentials, sandbox.CredentialsExplicit) {
		t.Run("credentials/explicit", c.credentialsExplicit)
	}
	if c.supports(sandbox.AxisNetwork, sandbox.NetworkNone) {
		t.Run("network/none", func(t *testing.T) { c.network(t, sandbox.NetworkNone) })
	}
	if c.supports(sandbox.AxisNetwork, sandbox.NetworkOpen) {
		t.Run("network/open", func(t *testing.T) { c.network(t, sandbox.NetworkOpen) })
	}
	if c.supports(sandbox.AxisResources, sandbox.ResourcesLimited) {
		t.Run("resources/limited memory", c.memoryLimited)
		t.Run("resources/limited tasks", c.tasksLimited)
	}
	if c.supports(sandbox.AxisNetwork, sandbox.NetworkAllowlist) {
		t.Run("network/allowlist", c.networkAllowlist)
	}
}

type suite struct {
	t    *testing.T
	info sandbox.Info
}

func (c *suite) supports(a sandbox.Axis, l sandbox.Level) bool {
	return slices.Contains(c.info.Supports[a], l)
}

// policy is the first level the runtime lists on every axis, with home and
// mounts for a confined storage axis.
func (c *suite) policy(t *testing.T, mounts ...sandbox.Mount) sandbox.Policy {
	first := func(a sandbox.Axis) sandbox.Level {
		if ls := c.info.Supports[a]; len(ls) > 0 {
			return ls[0]
		}
		return ""
	}
	home := t.TempDir()
	return sandbox.Policy{
		Storage:     sandbox.Storage{Level: first(sandbox.AxisStorage), Home: home, Mounts: mounts},
		Network:     sandbox.Network{Level: first(sandbox.AxisNetwork)},
		Exec:        sandbox.Exec{Level: first(sandbox.AxisExec)},
		Credentials: sandbox.Credentials{Level: first(sandbox.AxisCredentials), Env: []string{"PATH"}},
		Resources:   sandbox.Resources{Level: first(sandbox.AxisResources)},
	}
}

var hostPath = "PATH=/usr/local/bin:/usr/bin:/bin"

// run runs script with sh in the sandbox and returns its stdout.
func (c *suite) run(t *testing.T, p sandbox.Policy, script string, env ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	proc, err := sandbox.Start(ctx, c.info.Name, sandbox.Cmd{Args: []string{"sh", "-c", script}, Env: append([]string{hostPath}, env...)}, p)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = proc.Stdin().Close()
	out, _ := io.ReadAll(proc.Stdout())
	_ = proc.Wait()
	return string(out)
}

func (c *suite) refuses(t *testing.T) {
	p := c.policy(t)
	asks := map[sandbox.Axis]sandbox.Level{}
	for _, a := range sandbox.Axes {
		for _, l := range allLevels[a] {
			if !c.supports(a, l) {
				asks[a] = l
			}
		}
	}
	if len(asks) == 0 {
		t.Skip("the runtime gives every level")
	}
	for a, l := range asks {
		q := p
		switch a {
		case sandbox.AxisStorage:
			q.Storage.Level = l
		case sandbox.AxisNetwork:
			q.Network.Level = l
		case sandbox.AxisExec:
			q.Exec.Level = l
		case sandbox.AxisCredentials:
			q.Credentials.Level = l
		case sandbox.AxisResources:
			q.Resources.Level = l
		}
		_, err := sandbox.Start(context.Background(), c.info.Name, sandbox.Cmd{Args: []string{"true"}}, q)
		if !errors.Is(err, sandbox.ErrUnavailable) {
			t.Errorf("%s=%s is not given, and start must refuse; got %v", a, l, err)
		}
	}
}

var allLevels = map[sandbox.Axis][]sandbox.Level{
	sandbox.AxisStorage:     {sandbox.StorageHost, sandbox.StorageConfined},
	sandbox.AxisNetwork:     {sandbox.NetworkOpen, sandbox.NetworkNone, sandbox.NetworkAllowlist},
	sandbox.AxisExec:        {sandbox.ExecHost, sandbox.ExecConfined},
	sandbox.AxisCredentials: {sandbox.CredentialsInherit, sandbox.CredentialsExplicit},
	sandbox.AxisResources:   {sandbox.ResourcesUnlimited, sandbox.ResourcesLimited},
}

func (c *suite) storageConfined(t *testing.T) {
	root := t.TempDir()
	work, secret, ro := filepath.Join(root, "work"), filepath.Join(root, "secret"), filepath.Join(root, "work", "ro")
	for _, d := range []string{work, secret, ro} {
		must(t, os.MkdirAll(d, 0o755))
	}
	must(t, os.WriteFile(filepath.Join(secret, "key"), []byte("s3cret"), 0o600))
	must(t, os.WriteFile(filepath.Join(ro, "config"), []byte("orig"), 0o644))
	p := c.policy(t, sandbox.Mount{Path: work, Writable: true}, sandbox.Mount{Path: ro}, sandbox.Mount{Path: filepath.Join(root, "absent"), Optional: true})
	p.Storage.Level = sandbox.StorageConfined
	out := c.run(t, p, fmt.Sprintf(`
cat %[1]s/key 2>/dev/null && echo LEAK-unmounted
[ -e %[2]s ] && echo LEAK-dir
echo x > %[3]s/written && echo WROTE
echo x >> %[4]s/config 2>/dev/null && echo LEAK-readonly
[ "$HOME" = %[5]q ] && [ -z "$(ls -A "$HOME")" ] && echo HOME-OK
echo END`, secret, secret, work, ro, p.Storage.Home))
	expect(t, out, "WROTE", "HOME-OK", "END")
	if b, _ := os.ReadFile(filepath.Join(work, "written")); string(b) != "x\n" {
		t.Errorf("a write to a writable mount must land on the host, got %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ro, "config")); string(b) != "orig" {
		t.Errorf("a read-only mount changed: %q", b)
	}
	// The real home: not there, or not the host's.
	if home, err := os.UserHomeDir(); err == nil {
		out = c.run(t, p, fmt.Sprintf(`ls -A %q 2>/dev/null | grep -vx -e . | head -3; echo END`, home))
		for _, name := range []string{".ssh", ".bashrc", ".config"} {
			if _, err := os.Stat(filepath.Join(home, name)); err == nil && strings.Contains(out, name) {
				t.Errorf("the host's home is visible: %s", name)
			}
		}
	}
}

func (c *suite) execConfined(t *testing.T) {
	p := c.policy(t)
	p.Exec.Level = sandbox.ExecConfined
	out := c.run(t, p, `
grep -q '^CapEff:[[:space:]]*0*$' /proc/self/status && echo NO-CAPS
grep -q '^NoNewPrivs:[[:space:]]*1' /proc/self/status && echo NO-NEW-PRIVS
n=$(ls /proc | grep -c '^[0-9]'); [ "$n" -lt 20 ] && echo FEW-PIDS
(unshare -Ur true || unshare -m true) 2>/dev/null && echo LEAK-namespaces
echo END`)
	expect(t, out, "NO-CAPS", "NO-NEW-PRIVS", "FEW-PIDS", "END")
}

func (c *suite) killEndsEverything(t *testing.T) {
	root := t.TempDir()
	tick := filepath.Join(root, "tick")
	p := c.policy(t, sandbox.Mount{Path: root, Writable: true})
	p.Exec.Level = sandbox.ExecConfined
	proc, err := sandbox.Start(context.Background(), c.info.Name, sandbox.Cmd{
		Args: []string{"sh", "-c", `(while true; do date +%s%N >> ` + tick + `; sleep 0.05; done) & sleep 300`},
		Env:  []string{hostPath}}, p)
	must(t, err)
	time.Sleep(500 * time.Millisecond)
	must(t, proc.Kill())
	_ = proc.Wait()
	time.Sleep(300 * time.Millisecond)
	before, _ := os.ReadFile(tick)
	time.Sleep(400 * time.Millisecond)
	after, _ := os.ReadFile(tick)
	if len(before) == 0 {
		t.Fatal("the background process never ran")
	}
	if len(after) != len(before) {
		t.Error("a background process outlived the kill")
	}
}

func (c *suite) credentialsExplicit(t *testing.T) {
	p := c.policy(t)
	p.Credentials = sandbox.Credentials{Level: sandbox.CredentialsExplicit, Env: []string{"PATH", "KEEP_*"}}
	out := c.run(t, p, `[ -n "$GH_TOKEN" ] && echo LEAK-env; [ "$KEEP_ME" = 1 ] && echo KEPT; env | grep -c . ; echo END`,
		"GH_TOKEN=secret", "KEEP_ME=1", "AWS_SECRET_ACCESS_KEY=x")
	expect(t, out, "KEPT", "END")
}

// network tries to reach a listener on the host's loopback: none must fail,
// open must succeed.
func (c *suite) network(t *testing.T, level sandbox.Level) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("the probe needs bash (/dev/tcp)")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	p := c.policy(t)
	p.Network.Level = level
	port := ln.Addr().(*net.TCPAddr).Port
	out := c.run(t, p, fmt.Sprintf(`%s -c 'echo > /dev/tcp/127.0.0.1/%d' 2>/dev/null && echo REACHED; echo END`, bash, port))
	expect(t, out, "END")
	switch reached := strings.Contains(out, "REACHED"); {
	case level == sandbox.NetworkNone && reached:
		t.Error("network=none reached the host's loopback")
	case level == sandbox.NetworkOpen && !reached:
		t.Error("network=open could not reach the host")
	}
}

// limited is a policy with the resources axis limited, or skips the test where
// the host cannot give limits (the start is refused as unavailable).
func (c *suite) limited(t *testing.T, r sandbox.Resources) sandbox.Policy {
	p := c.policy(t)
	r.Level = sandbox.ResourcesLimited
	p.Resources = r
	proc, err := sandbox.Start(context.Background(), c.info.Name, sandbox.Cmd{Args: []string{"true"}, Env: []string{hostPath}}, p)
	if errors.Is(err, sandbox.ErrUnavailable) {
		t.Skipf("resource limits are not available here: %v", err)
	}
	must(t, err)
	_ = proc.Stdin().Close()
	_ = proc.Wait()
	return p
}

// memoryLimited sorts one 200 MB line under a 64 MiB limit: it needs the whole
// line in memory, so it must be killed (the scope may take the shell with it)
// before it finishes; a small one must still work.
func (c *suite) memoryLimited(t *testing.T) {
	p := c.limited(t, sandbox.Resources{MemoryBytes: 64 << 20})
	out := c.run(t, p, `[ -z "$XDG_RUNTIME_DIR$DBUS_SESSION_BUS_ADDRESS" ] || echo LEAK-session-env
head -c 8000000 /dev/zero | tr '\0' a | sort | wc -c | grep -q '^80000' && echo SMALL-OK
head -c 200000000 /dev/zero | tr '\0' a | sort | wc -c | grep -q '^2000' && echo LEAK-allocated`)
	expect(t, out, "SMALL-OK")
}

// tasksLimited forks past a limit of 20 tasks: some fork must fail.
func (c *suite) tasksLimited(t *testing.T) {
	p := c.limited(t, sandbox.Resources{Tasks: 20})
	out := c.run(t, p, `(n=0; while [ $n -lt 60 ]; do sleep 5 >/dev/null 2>&1 & n=$((n+1)); done) 2>/dev/null; echo STARTED=$(ls /proc | grep -c '^[0-9]'); echo END`)
	expect(t, out, "END")
	var started int
	i := strings.Index(out, "STARTED=")
	if i < 0 {
		t.Fatalf("no count in:\n%s", out)
	}
	_, _ = fmt.Sscanf(out[i:], "STARTED=%d", &started)
	if started >= 60 {
		t.Errorf("tasks=20 let 60 processes start")
	}
}

// networkAllowlist allows one of two listeners on the host's loopback: the
// proxy named by HTTPS_PROXY must tunnel to it and refuse the other, and
// neither may be reached directly.
func (c *suite) networkAllowlist(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("the probe needs bash (/dev/tcp)")
	}
	listen := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		must(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				_, _ = conn.Write([]byte("HELLO\n"))
				_ = conn.Close()
			}
		}()
		return ln.Addr().(*net.TCPAddr).Port
	}
	allowed, denied := listen(), listen()
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.sh")
	must(t, os.WriteFile(probe, []byte(fmt.Sprintf(`
[ -n "$HTTPS_PROXY" ] || echo NO-PROXY
hp=${HTTPS_PROXY#http://}; hp=${hp%%/}
via() {
	exec 3<>/dev/tcp/${hp%%:*}/${hp##*:} || return
	printf 'CONNECT 127.0.0.1:%%s HTTP/1.1\r\nHost: 127.0.0.1:%%s\r\n\r\n' $1 $1 >&3
	head -c 200 <&3 | tr '\r\n' '  '
	exec 3<&-
}
echo "ALLOWED[$(via %d)]"
echo "DENIED[$(via %d)]"
(echo > /dev/tcp/127.0.0.1/%d) 2>/dev/null && echo LEAK-direct
echo END
`, allowed, denied, allowed)), 0o644))
	p := c.policy(t, sandbox.Mount{Path: dir})
	p.Network = sandbox.Network{Level: sandbox.NetworkAllowlist, Allow: []string{fmt.Sprintf("127.0.0.1:%d", allowed)}}
	out := c.run(t, p, bash+" "+probe)
	expect(t, out, "END")
	if strings.Contains(out, "NO-PROXY") {
		t.Fatalf("network=allowlist set no HTTPS_PROXY:\n%s", out)
	}
	if !strings.Contains(out, "ALLOWED[HTTP/1.1 200") || !strings.Contains(out, "HELLO") {
		t.Errorf("the proxy did not tunnel to an allowed host:\n%s", out)
	}
	if !strings.Contains(out, "DENIED[HTTP/1.1 403") {
		t.Errorf("the proxy did not refuse a host off the list:\n%s", out)
	}
}

func expect(t *testing.T, out string, want ...string) {
	t.Helper()
	if strings.Contains(out, "LEAK") {
		t.Errorf("the sandbox gave away more than asked:\n%s", out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
