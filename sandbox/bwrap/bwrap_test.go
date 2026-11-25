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
