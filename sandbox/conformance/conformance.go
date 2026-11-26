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
		t.Run("resources/accounting", c.accounting)
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
		t.Run("resources/limited cpu", c.cpuLimited)
	}
	if c.supports(sandbox.AxisNetwork, sandbox.NetworkAllowlist) {
		t.Run("network/allowlist", c.networkAllowlist)
	}
	if c.supports(sandbox.AxisNetwork, sandbox.NetworkInternet) {
		t.Run("network/internet", c.networkInternet)
	}
	for _, l := range []sandbox.Level{sandbox.NetworkNone, sandbox.NetworkAllowlist, sandbox.NetworkInternet} {
		if c.supports(sandbox.AxisNetwork, l) {
			t.Run("network/"+string(l)+" expose", func(t *testing.T) { c.networkExpose(t, l) })
		}
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
	out, _ := c.runEnd(t, p, script, env...)
	return out
}

// runEnd is run, with how the process ended.
func (c *suite) runEnd(t *testing.T, p sandbox.Policy, script string, env ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	proc, err := sandbox.Start(ctx, c.info.Name, sandbox.Cmd{Args: []string{"sh", "-c", script}, Env: append([]string{hostPath}, env...)}, p)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = proc.Stdin().Close()
	out, _ := io.ReadAll(proc.Stdout())
	return string(out), proc.Wait()
}

func (c *suite) refuses(t *testing.T) {
	p := c.policy(t)
	asks := map[sandbox.Axis]sandbox.Level{}
	for _, a := range sandbox.Axes {
		for _, l := range sandbox.AxisLevels[a] {
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
	// A host process the sandbox must not see; the script names it in two
	// halves so it does not find itself.
	mark := fmt.Sprintf("tb-conformance-%d", time.Now().UnixNano())
	host := exec.Command("sh", "-c", "sleep 60 # "+mark)
	must(t, host.Start())
	defer func() { _ = host.Process.Kill(); _ = host.Wait() }()
	half := len(mark) / 2
	p := c.policy(t)
	p.Exec.Level = sandbox.ExecConfined
	out := c.run(t, p, fmt.Sprintf(`
grep -q '^CapEff:[[:space:]]*0*$' /proc/self/status && echo NO-CAPS
grep -q '^NoNewPrivs:[[:space:]]*1' /proc/self/status && echo NO-NEW-PRIVS
a=%q; b=%q; grep -qs "$a$b" /proc/[0-9]*/cmdline && echo LEAK-host-process
(unshare -Ur true || unshare -m true) 2>/dev/null && echo LEAK-namespaces
echo END`, mark[:half], mark[half:]))
	expect(t, out, "NO-CAPS", "NO-NEW-PRIVS", "END")
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
	out, err := c.runEnd(t, p, `[ -z "$XDG_RUNTIME_DIR$DBUS_SESSION_BUS_ADDRESS" ] || echo LEAK-session-env
head -c 8000000 /dev/zero | tr '\0' a | sort | wc -c | grep -q '^80000' && echo SMALL-OK
head -c 200000000 /dev/zero | tr '\0' a | sort | wc -c | grep -q '^2000' && echo LEAK-allocated`)
	expect(t, out, "SMALL-OK")
	var end *sandbox.ExitError
	if !errors.As(err, &end) || end.Reason != sandbox.EndOOM {
		t.Errorf("a sandbox that ran out of memory must end as oom, got %v", err)
	}
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
// cpuLimited spins for two seconds under a tenth of a core: the CPU time it
// gets must be well under the wall time.
func (c *suite) cpuLimited(t *testing.T) {
	p := c.limited(t, sandbox.Resources{CPU: 100})
	m, run := probe(t, `while [ $SECONDS -lt 2 ]; do :; done
read -r -a f < /proc/$$/stat; echo "TICKS=$(( f[13] + f[14] )) HZ=$(getconf CLK_TCK)"`)
	p.Storage.Mounts = append(p.Storage.Mounts, m)
	out := c.run(t, p, run)
	var ticks, hz int
	if i := strings.Index(out, "TICKS="); i < 0 {
		t.Fatalf("no count in:\n%s", out)
	} else {
		_, _ = fmt.Sscanf(out[i:], "TICKS=%d HZ=%d", &ticks, &hz)
	}
	if hz == 0 || float64(ticks)/float64(hz) > 0.6 {
		t.Errorf("cpu=100 (a tenth of a core) spun %d ticks at %d Hz in 2s", ticks, hz)
	}
}

// accounting: a Proc that reports usage reports this sandbox's.
func (c *suite) accounting(t *testing.T) {
	p := c.policy(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	proc, err := sandbox.Start(ctx, c.info.Name, sandbox.Cmd{Args: []string{"sh", "-c", "sleep 1 & sleep 1 & wait"}, Env: []string{hostPath}}, p)
	must(t, err)
	defer func() { _ = proc.Kill(); _ = proc.Wait() }()
	u, ok := proc.(sandbox.Usage)
	if !ok {
		t.Skip("the runtime does not report usage")
	}
	time.Sleep(300 * time.Millisecond)
	st, err := u.Usage()
	must(t, err)
	if st.MemoryBytes <= 0 || st.Tasks < 3 {
		t.Errorf("usage of a sandbox running three processes: %+v", st)
	}
}

// hostListener serves HELLO on the host's loopback; it returns the port.
func hostListener(t *testing.T) int {
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

// probe writes a bash script where the sandbox can read it, and returns the
// mount and the command that runs it.
func probe(t *testing.T, script string) (sandbox.Mount, string) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("the probe needs bash (/dev/tcp)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.sh")
	must(t, os.WriteFile(path, []byte(script), 0o644))
	return sandbox.Mount{Path: dir}, bash + " " + path
}

// networkAllowlist: the proxy named by HTTPS_PROXY refuses the host, even
// when the allowlist names it (by a name that resolves to it), and refuses
// what the list does not name; nothing is reached directly. Naming the host
// literally is not a policy at all. With a network, a listed public host is
// reached (the lower bound).
func (c *suite) networkAllowlist(t *testing.T) {
	port := hostListener(t)
	p := c.policy(t)
	p.Network = sandbox.Network{Level: sandbox.NetworkAllowlist, Allow: []string{fmt.Sprintf("127.0.0.1:%d", port)}}
	if _, err := sandbox.Start(context.Background(), c.info.Name, sandbox.Cmd{Args: []string{"true"}}, p); !errors.Is(err, sandbox.ErrUnavailable) {
		t.Errorf("an allowlist naming the host's loopback must not start; got %v", err)
	}

	// The host's own name resolves to the host (nss-myhostname and the like).
	self, _ := os.Hostname()
	if addrs, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip", self); err != nil || len(addrs) == 0 || sandbox.Classify(addrs[0]) != sandbox.SurfaceHost {
		self = ""
	}
	online := false
	if conn, err := net.DialTimeout("tcp", "example.com:443", 3*time.Second); err == nil {
		online = true
		_ = conn.Close()
	}
	allow := []string{"example.com"}
	if self != "" {
		allow = append(allow, fmt.Sprintf("%s:%d", self, port))
	}
	m, run := probe(t, fmt.Sprintf(`
[ -n "$HTTPS_PROXY" ] || echo NO-PROXY
hp=${HTTPS_PROXY#http://}; hp=${hp%%%%/}
via() {
	exec 3<>/dev/tcp/${hp%%%%:*}/${hp##*:} || return
	printf 'CONNECT %%s HTTP/1.1\r\nHost: %%s\r\n\r\n' $1 $1 >&3
	head -c 12 <&3
	exec 3<&-
}
echo "LOOPBACK[$(via 127.0.0.1:%[1]d)]"
[ -n %[2]q ] && echo "SELF[$(via %[2]s:%[1]d)]"
[ %[3]t = true ] && echo "PUBLIC[$(via example.com:443)]"
(echo > /dev/tcp/127.0.0.1/%[1]d) 2>/dev/null && echo LEAK-direct
echo END
`, port, self, online))
	p = c.policy(t, m)
	p.Network = sandbox.Network{Level: sandbox.NetworkAllowlist, Allow: allow}
	out := c.run(t, p, run)
	expect(t, out, "END", "LOOPBACK[HTTP/1.1 403")
	if strings.Contains(out, "NO-PROXY") {
		t.Fatalf("network=allowlist set no HTTPS_PROXY:\n%s", out)
	}
	if self != "" && !strings.Contains(out, "SELF[HTTP/1.1 403") {
		t.Errorf("a listed name that resolves to the host was not refused:\n%s", out)
	}
	if online && !strings.Contains(out, "PUBLIC[HTTP/1.1 200") {
		t.Errorf("a listed public host was not reached:\n%s", out)
	}
	if !online {
		t.Log("offline: the lower bound (a listed public host is reached) is not checked")
	}
}

// networkInternet: the upper bound, a listener on every address of the host,
// its loopback included, is unreachable; the lower
// bound, with a network, a public host is reached by name.
func (c *suite) networkInternet(t *testing.T) {
	p := c.policy(t)
	p.Network = sandbox.Network{Level: sandbox.NetworkInternet}
	proc, err := sandbox.Start(context.Background(), c.info.Name, sandbox.Cmd{Args: []string{"true"}, Env: []string{hostPath}}, p)
	if errors.Is(err, sandbox.ErrUnavailable) {
		t.Skipf("the internet level is not available here: %v", err)
	}
	must(t, err)
	_ = proc.Stdin().Close()
	_ = proc.Wait()

	ln, err := net.Listen("tcp", ":0")
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
	port := ln.Addr().(*net.TCPAddr).Port
	// The host's loopback too: a runtime must not forward it (pasta does,
	// unless told not to).
	targets := []string{"127.0.0.1"}
	for _, a := range sandbox.HostAddrs() {
		if !a.IsLoopback() && !a.IsLinkLocalUnicast() {
			targets = append(targets, a.String())
		}
	}
	online := false
	if conn, err := net.DialTimeout("tcp", "example.com:443", 3*time.Second); err == nil {
		online = true
		_ = conn.Close()
	}
	m, run := probe(t, fmt.Sprintf(`
for a in %[1]s; do
	timeout 3 bash -c "echo > /dev/tcp/$a/%[2]d" 2>/dev/null && echo "LEAK-host[$a]"
done
[ %[3]t = true ] && timeout 10 bash -c 'echo > /dev/tcp/example.com/443' && echo PUBLIC
echo END
`, strings.Join(targets, " "), port, online))
	p = c.policy(t, m)
	p.Network = sandbox.Network{Level: sandbox.NetworkInternet}
	out := c.run(t, p, run)
	expect(t, out, "END")
	if online && !strings.Contains(out, "PUBLIC") {
		t.Errorf("network=internet did not reach a public host by name:\n%s", out)
	}
	t.Logf("host addresses tried: %v; online: %v", targets, online)
}

// networkExpose: below open, an exposed port of the host's loopback is at
// localhost in the sandbox, by address and by name; another port is not.
func (c *suite) networkExpose(t *testing.T, level sandbox.Level) {
	exposed, other := hostListener(t), hostListener(t)
	m, run := probe(t, fmt.Sprintf(`
read -r a < /dev/tcp/127.0.0.1/%[1]d && echo "ADDR[$a]"
read -r b < /dev/tcp/localhost/%[1]d && echo "NAME[$b]"
(read -r c < /dev/tcp/127.0.0.1/%[2]d && echo "LEAK-other[$c]") 2>/dev/null
echo END
`, exposed, other))
	p := c.policy(t, m)
	p.Network = sandbox.Network{Level: level, Expose: []sandbox.Endpoint{{Port: exposed}}}
	out := c.run(t, p, run)
	expect(t, out, "ADDR[HELLO]", "NAME[HELLO]", "END")
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
