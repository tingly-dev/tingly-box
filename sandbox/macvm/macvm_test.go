//go:build darwin

package macvm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/conformance"
	"github.com/tingly-dev/tingly-box/sandbox/vmguest"
)

// The conformance suite needs a Mac with a guest image; elsewhere Check fails
// and it skips.
func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }

// fakeHV is a hypervisor whose machines run a Go function as the guest, which
// speaks the vsock protocol over in-memory connections.
type fakeHV struct {
	guest  func(g *fakeGuest)
	boots  chan vmConfig
	checks error
}

func (h *fakeHV) Check() error       { return h.checks }
func (h *fakeHV) Limits() (int, int) { return 8, 16384 }
func (h *fakeHV) New(c vmConfig) (vm, error) {
	if h.boots != nil {
		h.boots <- c
	}
	return &fakeVM{hv: h, ports: map[uint32]*fakeListener{}, done: make(chan struct{})}, nil
}

type fakeVM struct {
	hv    *fakeHV
	mu    sync.Mutex
	ports map[uint32]*fakeListener
	conns []net.Conn
	done  chan struct{}
	once  sync.Once
}

func (v *fakeVM) Listen(port uint32) (net.Listener, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	ln := &fakeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
	v.ports[port] = ln
	return ln, nil
}
func (v *fakeVM) Start() error {
	go func() {
		v.hv.guest(&fakeGuest{vm: v})
		v.Stop()
	}()
	return nil
}
func (v *fakeVM) Stop() error {
	v.once.Do(func() {
		close(v.done)
		// A stopped machine's vsock connections end.
		v.mu.Lock()
		defer v.mu.Unlock()
		for _, c := range v.conns {
			_ = c.Close()
		}
	})
	return nil
}
func (v *fakeVM) Done() <-chan struct{} { return v.done }

type fakeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func (l *fakeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *fakeListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *fakeListener) Addr() net.Addr { return &net.UnixAddr{Name: "fake", Net: "vsock"} }

// fakeGuest is the guest's side: it dials the host's ports.
type fakeGuest struct{ vm *fakeVM }

func (g *fakeGuest) dial(port int) net.Conn {
	g.vm.mu.Lock()
	ln := g.vm.ports[uint32(port)]
	g.vm.mu.Unlock()
	a, b := net.Pipe()
	g.vm.mu.Lock()
	g.vm.conns = append(g.vm.conns, b)
	g.vm.mu.Unlock()
	select {
	case ln.conns <- a:
	case <-time.After(5 * time.Second):
		panic("the host does not listen on port")
	}
	return b
}

// run is a guest that takes its command, writes out to stdout and ends with r.
func (g *fakeGuest) run(out string, r vmguest.Result, got *vmguest.Spec) {
	ctl := g.dial(vmguest.PortControl)
	line, _ := bufio.NewReader(ctl).ReadBytes('\n')
	_ = json.Unmarshal(line, got)
	stdin, stdout, stderr := g.dial(vmguest.PortStdin), g.dial(vmguest.PortStdout), g.dial(vmguest.PortStderr)
	if out != "" { // a write to a pipe waits for its reader, even an empty one
		_, _ = io.WriteString(stdout, out)
	}
	_ = stdout.Close()
	_ = stderr.Close()
	_ = stdin.Close()
	_ = json.NewEncoder(ctl).Encode(r)
}

// setUp makes a Mac of this machine: the hypervisor, the OS and a guest image.
func setUp(t *testing.T, hv *fakeHV) {
	t.Helper()
	oldBackend, oldGoos, oldArch := backend, goos, goarch
	backend, goos, goarch = hv, "darwin", "arm64"
	t.Cleanup(func() { backend, goos, goarch = oldBackend, oldGoos, oldArch })
	writeImage(t, t.TempDir())
}

func writeImage(t *testing.T, dir string) {
	t.Helper()
	t.Setenv(ImageEnv, dir)
	blob := func(name, content string) Blob {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(content))
		return Blob{Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
	}
	m := Manifest{Version: imageVersion, Arch: goarch, Kernel: blob(kernelFile, "kernel"), Rootfs: blob(rootfsFile, "rootfs")}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, manifestFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func policy(home string, mounts []sandbox.Mount, n sandbox.Network, r sandbox.Resources) sandbox.Policy {
	return sandbox.Policy{
		Storage:     sandbox.Storage{Level: sandbox.StorageConfined, Home: home, Mounts: mounts},
		Network:     n,
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit, Env: []string{"PATH"}},
		Resources:   r,
	}
}

var (
	noNet     = sandbox.Network{Level: sandbox.NetworkNone}
	unlimited = sandbox.Resources{Level: sandbox.ResourcesUnlimited}
)

func startVM(t *testing.T, p sandbox.Policy) (sandbox.Proc, error) {
	t.Helper()
	return sandbox.Start(context.Background(), Name, sandbox.Cmd{Args: []string{"/Users/u/bin/claude", "-p"}, Env: []string{"PATH=/usr/bin", "SECRET=x"}, Dir: "/work"}, p)
}

func TestStartSendsTheGuestItsViewAndReturnsItsOutput(t *testing.T) {
	var got vmguest.Spec
	boots := make(chan vmConfig, 1)
	setUp(t, &fakeHV{boots: boots, guest: func(g *fakeGuest) { g.run("hello\n", vmguest.Result{}, &got) }})
	work, home := t.TempDir(), "/Users/u"
	cfgFile := filepath.Join(work, ".git-config")
	if err := os.WriteFile(cfgFile, []byte("[user]"), 0o644); err != nil {
		t.Fatal(err)
	}
	proc, err := startVM(t, policy(home, []sandbox.Mount{{Path: work, Writable: true}, {Path: cfgFile}, {Path: "/nonexistent/x", Optional: true}}, noNet, unlimited))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(proc.Stdout())
	if err := proc.Wait(); err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello\n" {
		t.Fatalf("stdout %q", out)
	}
	// What the guest was told.
	if !got.Root || !got.Foreign || got.Tag != shareTag || got.User == "" || got.Dir != "/work" {
		t.Fatalf("spec %+v", got)
	}
	if want := []string{home, "/tmp", "/run"}; strings.Join(got.Fresh, ",") != strings.Join(want, ",") {
		t.Fatalf("fresh %q, want %q", got.Fresh, want)
	}
	if len(got.Mounts) != 2 || got.Mounts[0].Path != work || got.Mounts[0].ReadOnly || got.Mounts[1].Path != cfgFile || !got.Mounts[1].ReadOnly {
		t.Fatalf("mounts %+v", got.Mounts)
	}
	for _, kv := range got.Env {
		if strings.HasPrefix(kv, "SECRET=") {
			t.Fatal("the guest got an environment variable the policy does not keep")
		}
	}
	if got.Net != nil || got.Limits != nil {
		t.Fatalf("no card and no limits were asked for: %+v", got)
	}
	// What the machine was made with: the image, and the shares' read-only
	// enforced by the host.
	cfg := <-boots
	if !strings.HasSuffix(cfg.Rootfs, rootfsFile) || len(cfg.Shares) != 2 || cfg.Shares[0].ReadOnly || !cfg.Shares[1].ReadOnly || cfg.Card != nil {
		t.Fatalf("config %+v", cfg)
	}
}

func TestEndsAreTheGuestsReasons(t *testing.T) {
	for name, c := range map[string]struct {
		r      vmguest.Result
		reason sandbox.End
		code   int
	}{
		"exit":   {vmguest.Result{Exit: 3}, sandbox.EndExited, 3},
		"signal": {vmguest.Result{Signal: 15}, sandbox.EndSignaled, 143},
		"oom":    {vmguest.Result{Signal: 9, OOM: true}, sandbox.EndOOM, 137},
		"failed": {vmguest.Result{Error: "no such command"}, sandbox.EndFailed, -1},
	} {
		var got vmguest.Spec
		setUp(t, &fakeHV{guest: func(g *fakeGuest) { g.run("", c.r, &got) }})
		proc, err := startVM(t, policy("/Users/u", nil, noNet, unlimited))
		if err != nil {
			t.Fatal(name, err)
		}
		var end *sandbox.ExitError
		if err := proc.Wait(); !errors.As(err, &end) || end.Reason != c.reason || end.Code != c.code {
			t.Errorf("%s: got %v, want %s %d", name, err, c.reason, c.code)
		}
	}
}

func TestKillStopsTheMachineAndEndsAsKilled(t *testing.T) {
	hold := make(chan struct{})
	var got vmguest.Spec
	setUp(t, &fakeHV{guest: func(g *fakeGuest) {
		ctl := g.dial(vmguest.PortControl)
		_, _ = bufio.NewReader(ctl).ReadBytes('\n')
		for _, p := range []int{vmguest.PortStdin, vmguest.PortStdout, vmguest.PortStderr} {
			g.dial(p)
		}
		<-hold
		_ = got
	}})
	defer close(hold)
	proc, err := startVM(t, policy("/Users/u", nil, noNet, unlimited))
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Kill()
	var end *sandbox.ExitError
	if err := proc.Wait(); !errors.As(err, &end) || end.Reason != sandbox.EndKilled {
		t.Fatalf("got %v", err)
	}
}

func TestLimitsGoToTheGuestAndSizeTheMachine(t *testing.T) {
	var got vmguest.Spec
	boots := make(chan vmConfig, 1)
	setUp(t, &fakeHV{boots: boots, guest: func(g *fakeGuest) { g.run("", vmguest.Result{}, &got) }})
	r := sandbox.Resources{Level: sandbox.ResourcesLimited, MemoryBytes: 2 << 30, CPU: 1500}
	proc, err := startVM(t, policy("/Users/u", nil, noNet, r))
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Wait()
	cfg := <-boots
	if got.Limits == nil || got.Limits.MemoryBytes != 2<<30 || cfg.CPUs != 2 || cfg.MemoryMB != 2048+guestOverheadMB {
		t.Fatalf("limits %+v, cpus %d, memory %d", got.Limits, cfg.CPUs, cfg.MemoryMB)
	}
}

func TestGuestSize(t *testing.T) {
	oldCPUs, oldMem := guestCPUs, guestMemoryMB
	guestCPUs, guestMemoryMB = 10, func() int { return 64 << 10 }
	defer func() { guestCPUs, guestMemoryMB = oldCPUs, oldMem }()
	if c, m := guestSize(unlimited, 8, 32<<10); c != 8 || m != 32<<10 {
		t.Errorf("unlimited is the host's, within the framework's maximum: %d, %d", c, m)
	}
	if c, m := guestSize(sandbox.Resources{Level: sandbox.ResourcesLimited, CPU: 100}, 0, 0); c != 1 || m != 64<<10 {
		t.Errorf("a tenth of a core is a core: %d, %d", c, m)
	}
}

func TestUnavailableSaysWhy(t *testing.T) {
	old, oldGoos := backend, goos
	defer func() { backend, goos = old, oldGoos }()
	goos, backend = "linux", nil
	err := Runtime{}.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runs on macOS only") || !strings.Contains(err.Error(), "microvm") {
		t.Fatalf("on Linux: %v", err)
	}
	goos = "darwin"
	if err := (Runtime{}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "CGO_ENABLED=1") {
		t.Fatalf("without cgo: %v", err)
	}
	backend = &fakeHV{checks: errors.New("not signed with the virtualization entitlement")}
	t.Setenv(ImageEnv, t.TempDir())
	if err := (Runtime{}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "entitlement") {
		t.Fatalf("unsigned: %v", err)
	}
	backend = &fakeHV{}
	if err := (Runtime{}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "no guest image") {
		t.Fatalf("without an image: %v", err)
	}
	_, err = (Runtime{}).Start(context.Background(), sandbox.Cmd{Args: []string{"true"}}, policy("/h", nil, noNet, unlimited))
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Start without an image: %v", err)
	}
}

func TestImageIsCheckedAgainstItsManifest(t *testing.T) {
	setUp(t, &fakeHV{})
	dir := os.Getenv(ImageEnv)
	img, err := LoadImage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := img.Verify(); err != nil {
		t.Fatal(err)
	}
	// Same size, other contents: only Verify sees it.
	if err := os.WriteFile(filepath.Join(dir, rootfsFile), []byte("rootfX"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImage(dir); err != nil {
		t.Fatal(err)
	}
	if err := img.Verify(); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Verify: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, rootfsFile), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImage(dir); err == nil || !strings.Contains(err.Error(), "manifest says") {
		t.Fatalf("LoadImage with a wrong size: %v", err)
	}
	goarch = "amd64"
	if _, err := LoadImage(dir); err == nil || !strings.Contains(err.Error(), "arm64") {
		t.Fatalf("LoadImage for another architecture: %v", err)
	}
}

func TestPlanMountsStagesFilesNotTheirNeighbours(t *testing.T) {
	dir, stage := t.TempDir(), t.TempDir()
	file := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	shares, mounts, err := planMounts([]sandbox.Mount{{Path: file}, {Path: dir, Writable: true}}, stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 2 || shares[0].Path == dir || !shares[0].ReadOnly || shares[1].Path != realpath(t, dir) || shares[1].ReadOnly {
		t.Fatalf("shares %+v", shares)
	}
	if _, err := os.Stat(filepath.Join(shares[0].Path, "credentials.json")); err == nil {
		t.Fatal("the file's neighbour is in its share")
	}
	a, _ := os.Stat(file)
	b, err := os.Stat(filepath.Join(shares[0].Path, "settings.json"))
	if err != nil || !os.SameFile(a, b) {
		t.Fatalf("the staged file is not the same inode: %v", err)
	}
	if mounts[0].Path != file || mounts[0].Source != "m0/settings.json" || !mounts[0].ReadOnly || mounts[1].Source != "m1" {
		t.Fatalf("mounts %+v", mounts)
	}
	if _, _, err := planMounts([]sandbox.Mount{{Path: filepath.Join(dir, "missing")}}, stage); err == nil {
		t.Fatal("a missing required mount is accepted")
	}
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInternetGivesTheGuestACardOnTheFilteredNetwork(t *testing.T) {
	if _, err := os.Stat("/etc/resolv.conf"); err != nil {
		t.Skip(err)
	}
	var got vmguest.Spec
	boots := make(chan vmConfig, 1)
	setUp(t, &fakeHV{boots: boots, guest: func(g *fakeGuest) { g.run("", vmguest.Result{}, &got) }})
	proc, err := startVM(t, policy("/Users/u", nil, sandbox.Network{Level: sandbox.NetworkInternet}, unlimited))
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Wait()
	cfg := <-boots
	if cfg.Card == nil || cfg.MAC != guestMAC || got.Net == nil || got.Net.Gateway != "10.0.2.1" || !strings.Contains(got.ResolvConf, "169.254.1.53") {
		t.Fatalf("card %v, spec %+v", cfg.Card, got)
	}
	// By name grants are the filter's to refuse: it only knows addresses.
	_, err = startVM(t, policy("/Users/u", nil, sandbox.Network{Level: sandbox.NetworkInternet, Allow: []string{"intranet.example.com"}}, unlimited))
	if !errors.Is(err, sandbox.ErrUnavailable) || !strings.Contains(err.Error(), "by address only") {
		t.Fatalf("a grant by name: %v", err)
	}
}
