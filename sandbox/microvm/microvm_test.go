package microvm

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"

	"github.com/tingly-dev/tingly-box/sandbox/conformance"
)

// Runs where KVM, Cloud Hypervisor, virtiofsd and a guest kernel are; name
// them with TB_SANDBOX_VMM, TB_SANDBOX_VIRTIOFSD and TB_SANDBOX_KERNEL when
// they are not on PATH or where Kata Containers puts them.
func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }

// The guest's memory is the sandbox's: running out of it ends the sandbox as
// oom (the conformance suite cannot, the runtime has no memory limit level).
func TestGuestOutOfMemoryEndsAsOOM(t *testing.T) {
	if err := (Runtime{}).Check(context.Background()); err != nil {
		t.Skip(err)
	}
	old := guestMemoryMB
	guestMemoryMB = 256
	defer func() { guestMemoryMB = old }()
	p := sandbox.Policy{
		Storage:     sandbox.Storage{Level: sandbox.StorageConfined, Home: t.TempDir()},
		Network:     sandbox.Network{Level: sandbox.NetworkNone},
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsInherit},
		Resources:   sandbox.Resources{Level: sandbox.ResourcesUnlimited},
	}
	proc, err := sandbox.Start(context.Background(), Name, sandbox.Cmd{
		Args: []string{"sh", "-c", "head -c 600000000 /dev/zero | tr '\\0' a | sort > /dev/null"},
		Env:  []string{"PATH=/usr/bin:/bin"}}, p)
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Stdin().Close()
	_, _ = io.Copy(io.Discard, proc.Stdout())
	var end *sandbox.ExitError
	if err := proc.Wait(); !errors.As(err, &end) || end.Reason != sandbox.EndOOM {
		t.Fatalf("got %v, want an oom end", err)
	}
}
