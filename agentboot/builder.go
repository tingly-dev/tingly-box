package agentboot

// WithCompletionFunc sets a function to be called on completion.
// Convenience method that wraps f in a CompletionCallback.
func (h *CompositeHandler) WithCompletionFunc(f func(result *CompletionResult)) *CompositeHandler {
	return h.SetCompletionCallback(&funcCompletionCallback{onComplete: f})
}

// WithErrorFunc sets a function to be called on error.
// If a streamer is already set, the function is layered on top; otherwise a
// no-op streamer is created so only the error hook fires.
func (h *CompositeHandler) WithErrorFunc(f func(err error)) *CompositeHandler {
	if h.streamer == nil {
		h.streamer = &errorOnlyStreamer{onError: f}
	} else {
		h.streamer = &errorWrapperStreamer{MessageStreamer: h.streamer, onError: f}
	}
	return h
}

// --- Helper types ---

// funcCompletionCallback adapts a function to CompletionCallback.
type funcCompletionCallback struct {
	onComplete func(result *CompletionResult)
}

func (f *funcCompletionCallback) OnComplete(result *CompletionResult) {
	if f.onComplete != nil {
		f.onComplete(result)
	}
}

// errorOnlyStreamer implements MessageStreamer with only error handling.
type errorOnlyStreamer struct {
	onError func(err error)
}

func (e *errorOnlyStreamer) OnMessage(msg interface{}) error { return nil }

func (e *errorOnlyStreamer) OnError(err error) {
	if e.onError != nil {
		e.onError(err)
	}
}

// errorWrapperStreamer wraps a MessageStreamer and adds an error hook.
type errorWrapperStreamer struct {
	MessageStreamer
	onError func(err error)
}

func (e *errorWrapperStreamer) OnError(err error) {
	if e.onError != nil {
		e.onError(err)
	}
	if e.MessageStreamer != nil {
		e.MessageStreamer.OnError(err)
	}
}

var _ CompletionCallback = (*funcCompletionCallback)(nil)
var _ MessageStreamer = (*errorOnlyStreamer)(nil)
var _ MessageStreamer = (*errorWrapperStreamer)(nil)
