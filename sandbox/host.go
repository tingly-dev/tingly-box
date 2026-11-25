package sandbox

import "context"

// host runs the process on the host: the reference for "no isolation". It is
// what a caller picks to run as before while still going through the same
// interface; only the credentials axis (applied by Start) can be tightened.
type host struct{}

// HostName is the name of the runtime that does not isolate.
const HostName = "host"

func init() { Register(host{}) }

func (host) Info() Info {
	return Info{Name: HostName, Isolation: IsolationNone, Supports: map[Axis][]Level{
		AxisStorage:     {StorageHost},
		AxisNetwork:     {NetworkOpen},
		AxisExec:        {ExecHost},
		AxisCredentials: {CredentialsInherit, CredentialsExplicit},
		AxisResources:   {ResourcesUnlimited},
	}}
}

func (host) Check(context.Context) error { return nil }

func (host) Start(ctx context.Context, cmd Cmd, _ Policy) (Proc, error) { return ExecCommand(ctx, cmd) }
