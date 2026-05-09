package streamemit

// EmissionPolicy controls when events for a given block kind are released
// from the emitter to the caller.
type EmissionPolicy int

const (
	// EmitImmediate releases each event as soon as it is fed to the emitter.
	// This is the zero value and matches today's passthrough behavior.
	EmitImmediate EmissionPolicy = iota

	// EmitOnComplete buffers all events for a single content block from
	// content_block_start through content_block_stop, then releases them
	// as one ordered slice when the stop event arrives.
	EmitOnComplete
)

// Config configures a StreamEmitter. The zero value emits everything
// immediately and installs no hooks.
type Config struct {
	// TextPolicy governs "text" content blocks.
	TextPolicy EmissionPolicy

	// ThinkingPolicy governs "thinking" and "redacted_thinking" content blocks.
	ThinkingPolicy EmissionPolicy

	// ToolPolicy governs "tool_use" content blocks. Setting this to
	// EmitOnComplete is the primary use case for this package: tool events
	// are held until the tool's content_block_stop arrives.
	ToolPolicy EmissionPolicy

	// OnToolBlockComplete is invoked when a tool_use block has finished
	// buffering under EmitOnComplete and is about to be released. The hook
	// receives the tool_use id, the content block index, and the buffered
	// events in arrival order. It may return a *ToolDecision to replace or
	// drop the buffered events; returning nil flushes them as-is. An error
	// short-circuits Feed and is returned to the caller.
	OnToolBlockComplete func(toolID string, index int, buffered []BufferedEvent) (*ToolDecision, error)
}

// ToolDecision is the return value of Config.OnToolBlockComplete.
//
// If Drop is true the buffered events are suppressed entirely.
// Otherwise, if Replace is non-nil those events are emitted instead of
// the buffered ones. A nil ToolDecision (or a zero-value one) means
// "flush the buffered events unchanged".
type ToolDecision struct {
	Replace []BufferedEvent
	Drop    bool
}
