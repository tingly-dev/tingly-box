package core

// Stable action labels used across command normalization and policy matching.
// Keep this vocabulary small so policy configuration stays predictable.
const (
	ActionRead    = "read"
	ActionWrite   = "write"
	ActionDelete  = "delete"
	ActionExecute = "execute"
	ActionNetwork = "network"
)
