package session

// LifecycleStore receives session lifecycle transitions from the agentboot runner.
// remote/session.Manager implements this interface; the interface lives here
// so the runner has no dependency on the bot or remote layers.
//
// The runner calls:
//   - SetRunning after the process starts successfully
//   - SetFailed if the process fails to start or Wait returns an error
//   - SetCompleted if Wait returns without error
type LifecycleStore interface {
	SetRunning(id string) bool
	SetCompleted(id, response string) bool
	SetFailed(id, errMsg string) bool
}

// Store is retained for source compatibility.
// Deprecated: use LifecycleStore.
type Store = LifecycleStore
