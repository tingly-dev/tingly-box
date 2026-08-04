package adapter

import (
	"github.com/tingly-dev/tingly-box/agentboot"
	remotesession "github.com/tingly-dev/tingly-box/remote/session"
)

// Compile-time assertion that remote/session.Manager satisfies the
// agentboot runner's LifecycleStore contract. This guard used to live in
// remote/session/manager.go but was lifted into this host↔remote bridge so
// remote/session stays a pure leaf with no dependency back into agentboot.
// If a refactor drops or renames any of SetRunning/SetCompleted/SetFailed
// the build fails here instead of the runner crashing at runtime.
var _ agentboot.LifecycleStore = (*remotesession.Manager)(nil)
