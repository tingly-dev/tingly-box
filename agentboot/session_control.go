package agentboot

import (
	"errors"
	"fmt"
)

// Session control is the backend-neutral surface Projects (and any other host)
// uses to steer a running agent session: compaction and context size so far.
// A backend implements the pieces it has as optional capabilities on its
// [AgentTransport]; a missing one makes the call return the matching
// Err…Unsupported. Nothing here names a particular agent's wire format.

// CompactTrigger says who started a compaction.
type CompactTrigger string

const (
	CompactManual CompactTrigger = "manual"
	CompactAuto   CompactTrigger = "auto"
)

// TokenUsage is a token count in the four buckets agents bill by.
type TokenUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

// CompactReported says which optional figures of a [CompactResult] the
// backend actually reported, so a zero can be told from "unknown".
type CompactReported struct {
	Tokens  bool `json:"tokens"`
	Summary bool `json:"summary"`
	Usage   bool `json:"usage"`
}

// CompactResult is what a finished compaction reports.
type CompactResult struct {
	Trigger      CompactTrigger  `json:"trigger,omitempty"`
	TokensBefore int64           `json:"tokens_before,omitempty"`
	TokensAfter  int64           `json:"tokens_after,omitempty"`
	DurationMS   int64           `json:"duration_ms,omitempty"`
	Summary      string          `json:"summary,omitempty"` // the summary text, when the backend exposes it
	Usage        *TokenUsage     `json:"usage,omitempty"`   // what the compaction call itself spent, when reported
	Reported     CompactReported `json:"reported"`
}

// Compact phases.
const (
	CompactStarted = "started"
	CompactDone    = "done"
	CompactFailed  = "failed"
)

// CompactEvent is the neutral event for a compaction, whether a host asked
// for it or the agent started it itself. A backend's transport emits it as a
// [MessageEvent].Raw value. Result is set when Phase is CompactDone, Reason
// when it is CompactFailed.
type CompactEvent struct {
	Phase  string        `json:"phase"`
	Result CompactResult `json:"result"`
	Reason string        `json:"reason,omitempty"`
}

// ContextUsage is how full the session's context is right now.
type ContextUsage struct {
	Used          int64 `json:"used"`
	Max           int64 `json:"max"`
	AutoCompactAt int64 `json:"auto_compact_at,omitempty"` // 0: the backend has no automatic compaction
}

var (
	// ErrCompactUnsupported: the session's backend cannot compact.
	ErrCompactUnsupported = errors.New("agentboot: agent does not support compaction")
	// ErrContextUnsupported: the session's backend cannot report context usage.
	ErrContextUnsupported = errors.New("agentboot: agent cannot report context usage")
	// ErrNoProcess: the session has no live process to ask.
	ErrNoProcess = errors.New("agentboot: session has no live process")
)

// CompactFailedError is the backend declining to compact ("Not enough
// messages to compact.").
type CompactFailedError struct{ Reason string }

func (e *CompactFailedError) Error() string {
	if e.Reason == "" {
		return "agentboot: compaction failed"
	}
	return fmt.Sprintf("agentboot: compaction failed: %s", e.Reason)
}

// Compactor is implemented by transports whose agent can compact its
// conversation on request. Optional. The compaction itself is an ordinary
// turn: the host sends CompactPrompt as the turn's prompt, and the transport
// reports the outcome as [CompactEvent] messages (also for compactions the
// agent starts on its own).
type Compactor interface {
	// CompactPrompt is the user message that makes the agent compact; the
	// optional instructions steer the summary.
	CompactPrompt(instructions string) string
}

// ContextUsageReader is implemented by transports whose agent answers a
// query for its context size. Optional. The query is a [ContextUsageRequest]
// control request, encoded by the transport's [ControlRequestEncoder].
type ContextUsageReader interface {
	// DecodeContextUsage reads the agent's answer body.
	DecodeContextUsage(body map[string]any) (ContextUsage, error)
}

// ContextUsageRequest asks the running process for its context size.
type ContextUsageRequest struct{}

func (ContextUsageRequest) isControlRequest() {}
