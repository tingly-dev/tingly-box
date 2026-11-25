package sandbox_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/conformance"
)

func TestHostConformance(t *testing.T) {
	rt, ok := sandbox.Get(sandbox.HostName)
	if !ok {
		t.Fatal("the host runtime is always registered")
	}
	conformance.Run(t, rt)
}

func TestValidateNamesEveryAxisItCannotGive(t *testing.T) {
	info := sandbox.Info{Name: "x", Supports: map[sandbox.Axis][]sandbox.Level{
		sandbox.AxisStorage: {sandbox.StorageConfined}, sandbox.AxisNetwork: {sandbox.NetworkOpen},
		sandbox.AxisExec: {sandbox.ExecConfined}, sandbox.AxisCredentials: {sandbox.CredentialsExplicit},
		sandbox.AxisResources: {sandbox.ResourcesUnlimited},
	}}
	p := sandbox.Policy{
		Storage: sandbox.Storage{Level: sandbox.StorageConfined}, Network: sandbox.Network{Level: sandbox.NetworkNone},
		Exec: sandbox.Exec{Level: sandbox.ExecConfined}, Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit},
	}
	err := sandbox.Validate(info, p)
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"network=none", "resources=(unset)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not name %s", err, want)
		}
	}
	p.Network.Level, p.Resources.Level = sandbox.NetworkOpen, sandbox.ResourcesUnlimited
	if err := sandbox.Validate(info, p); err != nil {
		t.Fatal(err)
	}
}

func TestStartRefusesAnUnknownRuntime(t *testing.T) {
	_, err := sandbox.Start(context.Background(), "nope", sandbox.Cmd{Args: []string{"true"}}, sandbox.Policy{})
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestFilterEnv(t *testing.T) {
	got := sandbox.FilterEnv([]string{"PATH=/bin", "HOME=/root", "ANTHROPIC_BASE_URL=u", "GH_TOKEN=x"}, []string{"PATH", "ANTHROPIC_*"}, "/h")
	slices.Sort(got)
	if want := []string{"ANTHROPIC_BASE_URL=u", "HOME=/h", "PATH=/bin"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
