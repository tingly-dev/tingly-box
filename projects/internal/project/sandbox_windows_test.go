package project

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
)

func TestWindowsSandboxUnavailable(t *testing.T) {
	if !ValidSandbox("") || !ValidSandbox(sandbox.HostName) {
		t.Fatal("host execution must remain available")
	}
	for _, name := range []string{"bwrap", "microvm", "macvm", "seatbelt"} {
		t.Run(name, func(t *testing.T) {
			if ValidSandbox(name) {
				t.Fatalf("unsupported runtime %s is registered on Windows", name)
			}
			if err := CheckThreadSandbox(context.Background(), name); err == nil {
				t.Fatal("unsupported sandbox passed the thread check")
			}
			// An existing project may still name a sandbox from another OS.
			// Refuse it before starting a process; never fall back to the host.
			proc, err := sandbox.Start(context.Background(), name, sandbox.Cmd{}, sandbox.Policy{})
			if proc != nil || !errors.Is(err, sandbox.ErrUnavailable) {
				t.Fatalf("Start = (%v, %v), want ErrUnavailable", proc, err)
			}
		})
	}
}
