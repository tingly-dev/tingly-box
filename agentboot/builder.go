package agentboot

// HandlerBuilder provides a fluent API for building CompositeHandler instances.
type HandlerBuilder struct {
	handler *CompositeHandler
}

// NewHandlerBuilder creates a new HandlerBuilder.
func NewHandlerBuilder() *HandlerBuilder {
	return &HandlerBuilder{
		handler: NewCompositeHandler(),
	}
}

// WithStreamer sets the message streamer handler.
func (b *HandlerBuilder) WithStreamer(s MessageStreamer) *HandlerBuilder {
	b.handler.SetStreamer(s)
	return b
}

// WithApprovalHandler sets the approval handler.
func (b *HandlerBuilder) WithApprovalHandler(a ApprovalHandler) *HandlerBuilder {
	b.handler.SetApprovalHandler(a)
	return b
}

// WithAskHandler sets the ask handler.
func (b *HandlerBuilder) WithAskHandler(a AskHandler) *HandlerBuilder {
	b.handler.SetAskHandler(a)
	return b
}

// WithCompletionCallback sets the completion callback.
func (b *HandlerBuilder) WithCompletionCallback(c CompletionCallback) *HandlerBuilder {
	b.handler.SetCompletionCallback(c)
	return b
}

// OnComplete sets a function to be called on completion.
// This is a convenience method that wraps the function in a CompletionCallback.
func (b *HandlerBuilder) OnComplete(f func(result *CompletionResult)) *HandlerBuilder {
	b.handler.SetCompletionCallback(&funcCompletionCallback{onComplete: f})
	return b
}

// OnError sets a function to be called on error.
// This wraps the existing streamer to also call the error function.
func (b *HandlerBuilder) OnError(f func(err error)) *HandlerBuilder {
	if b.handler.streamer == nil {
		b.handler.streamer = &errorOnlyStreamer{onError: f}
	} else {
		// Wrap existing streamer
		prev := b.handler.streamer
		b.handler.streamer = &errorWrapperStreamer{
			MessageStreamer: prev,
			onError:         f,
		}
	}
	return b
}

// Build returns the configured MessageHandler.
func (b *HandlerBuilder) Build() MessageHandler {
	return b.handler
}

// --- Helper types for builder ---

// funcCompletionCallback adapts a function to CompletionCallback.
type funcCompletionCallback struct {
	onComplete func(result *CompletionResult)
}

// OnComplete implements CompletionCallback.
func (f *funcCompletionCallback) OnComplete(result *CompletionResult) {
	if f.onComplete != nil {
		f.onComplete(result)
	}
}

// errorOnlyStreamer implements MessageStreamer with only error handling.
type errorOnlyStreamer struct {
	onError func(err error)
}

// OnMessage implements MessageStreamer.
func (e *errorOnlyStreamer) OnMessage(msg interface{}) error {
	return nil
}

// OnError implements MessageStreamer.
func (e *errorOnlyStreamer) OnError(err error) {
	if e.onError != nil {
		e.onError(err)
	}
}

// errorWrapperStreamer wraps a MessageStreamer and adds error handling.
type errorWrapperStreamer struct {
	MessageStreamer
	onError func(err error)
}

// OnError implements MessageStreamer.
func (e *errorWrapperStreamer) OnError(err error) {
	if e.onError != nil {
		e.onError(err)
	}
	if e.MessageStreamer != nil {
		e.MessageStreamer.OnError(err)
	}
}

// Ensure builder types implement interfaces
var _ CompletionCallback = (*funcCompletionCallback)(nil)
var _ MessageStreamer = (*errorOnlyStreamer)(nil)
var _ MessageStreamer = (*errorWrapperStreamer)(nil)
