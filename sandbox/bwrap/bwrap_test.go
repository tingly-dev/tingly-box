package bwrap

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/conformance"
)

func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }

func policy() sandbox.Policy {
	return sandbox.Policy{
		Storage: sandbox.Storage{Level: sandbox.StorageConfined, Home: "/home/u", Mounts: []sandbox.Mount{
			{Path: "/repo/.git/config"},
			{Path: "/repo", Writable: true},
			{Path: "/home/u/.claude/skills", Optional: true},
		}},
		Network:     sandbox.Network{Level: sandbox.NetworkOpen},
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit},
		Resources:   sandbox.Resources{Level: sandbox.ResourcesUnlimited},
	}
}

func TestArgsConfineToTheMounts(t *testing.T) {
	args, err := Args(policy(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(args, " ")
	for _, want := range []string{
		"--unshare-pid", "--disable-userns", "--cap-drop ALL", "--new-session", "--die-with-parent",
		"--ro-bind /usr /usr", "--tmpfs /home/u",
		"--bind /repo /repo", "--ro-bind /repo/.git/config /repo/.git/config", "--ro-bind-try /home/u/.claude/skills /home/u/.claude/skills",
		"--chdir /repo",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("args lack %q:\n%s", want, cmd)
		}
	}
	if !strings.HasSuffix(cmd, " --") {
		t.Error("args end with --")
	}
	if strings.Contains(cmd, "--unshare-net") {
		t.Error("network=open shares the host's network")
	}
	if strings.Index(cmd, "--bind /repo /repo") > strings.Index(cmd, "--ro-bind /repo/.git/config") {
		t.Error("a read-only file must be mounted after the directory it covers")
	}
	if strings.Index(cmd, "--tmpfs /home/u") > strings.Index(cmd, "/home/u/.claude/skills") {
		t.Error("mounts under HOME must come after its tmpfs")
	}

	p := policy()
	p.Network.Level = sandbox.NetworkNone
	args, _ = Args(p, "")
	if !strings.Contains(strings.Join(args, " "), "--unshare-net") {
		t.Error("network=none unshares the network")
	}
	p.Network.Level = sandbox.NetworkAllowlist
	args, _ = Args(p, "")
	if !strings.Contains(strings.Join(args, " "), "--unshare-net") {
		t.Error("network=allowlist has no network of its own")
	}
}

func TestScopeArgs(t *testing.T) {
	got := strings.Join(scopeArgs("/bin/systemd-run", sandbox.Resources{Level: sandbox.ResourcesLimited, MemoryBytes: 1 << 26, Tasks: 50}), " ")
	want := "/bin/systemd-run --user --scope --quiet --collect -p MemoryMax=67108864 -p MemorySwapMax=0 -p TasksMax=50 --"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	got = strings.Join(scopeArgs("sr", sandbox.Resources{Level: sandbox.ResourcesLimited, Tasks: 5}), " ")
	if strings.Contains(got, "Memory") || !strings.Contains(got, "TasksMax=5") {
		t.Errorf("a zero limit is left out: %s", got)
	}
}

func TestLimitedRefusesWithoutSystemdRun(t *testing.T) {
	old := lookSystemdRun
	lookSystemdRun = func() (string, error) { return "", exec.ErrNotFound }
	defer func() { lookSystemdRun = old }()
	p := policy()
	p.Resources = sandbox.Resources{Level: sandbox.ResourcesLimited, MemoryBytes: 1 << 26}
	_, err := Runtime{}.Start(context.Background(), sandbox.Cmd{Args: []string{"true"}}, p)
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable: never run without the limits", err)
	}
	if !strings.Contains(err.Error(), "systemd") {
		t.Errorf("the reason should name systemd: %v", err)
	}
}

func TestRefusesToStartWithoutBwrap(t *testing.T) {
	old := lookPath
	lookPath = func() (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = old }()
	_, err := sandbox.Start(context.Background(), Name, sandbox.Cmd{Args: []string{"true"}}, policy())
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable: never run unconfined", err)
	}
}
