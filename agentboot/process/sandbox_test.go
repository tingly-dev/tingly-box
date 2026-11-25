package process

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
)

func TestASandboxedLaunchGoesToItsRuntime(t *testing.T) {
	// The host runtime with explicit credentials: what reaches the process is
	// the runtime's doing, not the factory's.
	h, err := NewOSExecFactory().Start(context.Background(), LaunchSpec{
		Command: []string{"sh", "-c", `echo "token=$GH_TOKEN path=${PATH:+set}"`},
		Env:     []string{"PATH=/usr/bin:/bin", "GH_TOKEN=secret"},
		Sandbox: &Sandbox{Runtime: sandbox.HostName, Policy: sandbox.Policy{
			Storage:     sandbox.Storage{Level: sandbox.StorageHost},
			Network:     sandbox.Network{Level: sandbox.NetworkOpen},
			Exec:        sandbox.Exec{Level: sandbox.ExecHost},
			Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit, Env: []string{"PATH"}},
			Resources:   sandbox.Resources{Level: sandbox.ResourcesUnlimited},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = h.Stdin().Close()
	out, _ := io.ReadAll(h.Stdout())
	_ = h.Wait()
	if got := strings.TrimSpace(string(out)); got != "token= path=set" {
		t.Fatalf("got %q", got)
	}
}

func TestASandboxThatCannotBeMadeIsNeverStartedOnTheHost(t *testing.T) {
	_, err := NewOSExecFactory().Start(context.Background(), LaunchSpec{Command: []string{"true"}, Sandbox: &Sandbox{Runtime: "absent"}})
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("err = %v, want sandbox.ErrUnavailable", err)
	}
}
