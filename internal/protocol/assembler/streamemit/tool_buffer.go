package streamemit

// toolBlockBuffer accumulates the start, deltas, and stop events for a
// single tool_use content block while EmitOnComplete is in effect. It is
// flushed as one ordered slice when the block's content_block_stop event
// arrives (or when Drain is called).
type toolBlockBuffer struct {
	index  int
	toolID string
	events []BufferedEvent
}

func newToolBlockBuffer(index int, toolID string) *toolBlockBuffer {
	return &toolBlockBuffer{index: index, toolID: toolID}
}

func (b *toolBlockBuffer) append(evt BufferedEvent) {
	b.events = append(b.events, evt)
}

// drain returns all buffered events in arrival order and clears the buffer.
func (b *toolBlockBuffer) drain() []BufferedEvent {
	out := b.events
	b.events = nil
	return out
}

// snapshot returns a copy of the buffered events without clearing the buffer.
// Used by StreamEmitter.ToolBuffer for read-only inspection.
func (b *toolBlockBuffer) snapshot() []BufferedEvent {
	if len(b.events) == 0 {
		return nil
	}
	out := make([]BufferedEvent, len(b.events))
	copy(out, b.events)
	return out
}
