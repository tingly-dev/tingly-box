package seatbelt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/conformance"
)

// Runs on a Mac; elsewhere Check fails and the suite skips.
func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }

func policy(mounts []sandbox.Mount, n sandbox.Network) sandbox.Policy {
	return sandbox.Policy{
		Storage:     sandbox.Storage{Level: sandbox.StorageConfined, Home: "/Users/u", Mounts: mounts},
		Network:     n,
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit},
		Resources:   sandbox.Resources{Level: sandbox.ResourcesUnlimited},
	}
}

func TestProfileOrdersMountsByDepthSoTheLongestPathWins(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	cfg := filepath.Join(repo, ".git", "config")
	for _, d := range []string{filepath.Join(repo, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// The read-only file is listed first and still ends up after the writable
	// directory it lies in.
	got, err := profile(plan{policy: policy([]sandbox.Mount{{Path: cfg}, {Path: repo, Writable: true}, {Path: filepath.Join(repo, "gone"), Optional: true}},
		sandbox.Network{Level: sandbox.NetworkNone})})
	if err != nil {
		t.Fatal(err)
	}
	rw := strings.Index(got, `(allow file-read* file-write* (subpath "`+realpath(t, repo)+`"))`)
	ro := strings.Index(got, `(deny file-write* (literal "`+realpath(t, cfg)+`"))`)
	if rw < 0 || ro < 0 || ro < rw {
		t.Fatalf("the read-only file must come after the writable directory (rw at %d, ro at %d):\n%s", rw, ro, got)
	}
	if strings.Contains(got, "gone") {
		t.Fatal("an optional mount that does not exist is in the profile")
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

func TestProfileRequiredMountMustExist(t *testing.T) {
	_, err := profile(plan{policy: policy([]sandbox.Mount{{Path: "/nonexistent/tb"}}, sandbox.Network{Level: sandbox.NetworkNone})})
	if err == nil {
		t.Fatal("a missing required mount is accepted")
	}
}

func TestProfileNetwork(t *testing.T) {
	none, err := profile(plan{policy: policy(nil, sandbox.Network{Level: sandbox.NetworkNone, Expose: []sandbox.Endpoint{{Port: 8080}, {Port: 9000}}})})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none, "(allow network*)") || !strings.Contains(none, `(remote ip "localhost:8080")`) || !strings.Contains(none, `(remote ip "localhost:9000")`) {
		t.Fatalf("none with Expose:\n%s", none)
	}
	if strings.Contains(none, "network-bind") || strings.Contains(none, "network-inbound") {
		t.Fatal("the sandbox may listen")
	}
	allow, err := profile(plan{policy: policy(nil, sandbox.Network{Level: sandbox.NetworkAllowlist}), proxyPort: 4321})
	if err != nil || !strings.Contains(allow, `(remote ip "localhost:4321")`) {
		t.Fatalf("allowlist: %v\n%s", err, allow)
	}
	if _, err := profile(plan{policy: policy(nil, sandbox.Network{Level: sandbox.NetworkAllowlist})}); err == nil {
		t.Fatal("an allowlist without its proxy is accepted")
	}
	open, err := profile(plan{policy: policy(nil, sandbox.Network{Level: sandbox.NetworkOpen})})
	if err != nil || !strings.Contains(open, "(allow network*)") {
		t.Fatalf("open: %v\n%s", err, open)
	}
	if _, err := profile(plan{policy: policy(nil, sandbox.Network{Level: sandbox.NetworkInternet})}); err == nil {
		t.Fatal("the internet level is given by a profile")
	}
}

func TestQuoteEscapes(t *testing.T) {
	if got := quote("/a \"b\"\\c\nd"); got != `"/a \"b\"\\c\nd"` {
		t.Fatalf("got %s", got)
	}
}

func TestInternetAndLimitsAreUnavailable(t *testing.T) {
	for name, mutate := range map[string]func(*sandbox.Policy){
		"internet": func(p *sandbox.Policy) { p.Network.Level = sandbox.NetworkInternet },
		"limits": func(p *sandbox.Policy) {
			p.Resources = sandbox.Resources{Level: sandbox.ResourcesLimited, MemoryBytes: 1 << 30}
		},
	} {
		p := policy(nil, sandbox.Network{Level: sandbox.NetworkNone})
		mutate(&p)
		_, err := sandbox.Start(context.Background(), Name, sandbox.Cmd{Args: []string{"true"}}, p)
		if !errors.Is(err, sandbox.ErrUnavailable) || !strings.Contains(err.Error(), "cannot give") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMacOnly(t *testing.T) {
	old := goos
	goos = "linux"
	defer func() { goos = old }()
	err := Runtime{}.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "macOS only") {
		t.Fatalf("Check: %v", err)
	}
	_, err = Runtime{}.Start(context.Background(), sandbox.Cmd{Args: []string{"true"}}, policy(nil, sandbox.Network{Level: sandbox.NetworkNone}))
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Start: %v", err)
	}
}

func TestEnvironmentMovesHomeAndNamesTheProxy(t *testing.T) {
	got := environment([]string{"PATH=/bin", "HOME=/Users/u", "TMPDIR=/x"}, "/s/home", "/s/tmp", 4321)
	want := map[string]bool{"PATH=/bin": true, "HOME=/s/home": true, "TMPDIR=/s/tmp": true, "HTTP_PROXY=http://127.0.0.1:4321": true, "no_proxy=127.0.0.1,localhost": true}
	seen := map[string]bool{}
	for _, kv := range got {
		seen[kv] = true
		if kv == "HOME=/Users/u" || kv == "TMPDIR=/x" {
			t.Errorf("kept %s", kv)
		}
	}
	for kv := range want {
		if !seen[kv] {
			t.Errorf("missing %s in %q", kv, got)
		}
	}
}
