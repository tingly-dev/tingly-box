package agentboot

import (
	"context"
)

// CompositeHandler combines multiple handler interfaces into one MessageHandler.
// It allows composing different handlers for streaming, approval, ask, and completion.
type CompositeHandler struct {
	streamer  MessageStreamer
	approval  ApprovalHandler
	ask       AskHandler
	completer CompletionCallback
}

// NewCompositeHandler creates a new empty CompositeHandler.
// Use Set* methods to configure individual handlers.
func NewCompositeHandler() *CompositeHandler {
	return &CompositeHandler{}
}

// SetStreamer sets the message streamer handler.
// Returns self for chaining.
func (h *CompositeHandler) SetStreamer(s MessageStreamer) *CompositeHandler {
	h.streamer = s
	return h
}

// SetApprovalHandler sets the approval handler.
// Returns self for chaining.
func (h *CompositeHandler) SetApprovalHandler(a ApprovalHandler) *CompositeHandler {
	h.approval = a
	return h
}

// SetAskHandler sets the ask handler.
// Returns self for chaining.
func (h *CompositeHandler) SetAskHandler(a AskHandler) *CompositeHandler {
	h.ask = a
	return h
}

// SetCompletionCallback sets the completion callback.
// Returns self for chaining.
func (h *CompositeHandler) SetCompletionCallback(c CompletionCallback) *CompositeHandler {
	h.completer = c
	return h
}

// OnMessage implements MessageHandler.
// Forwards to the MessageStreamer if set.
func (h *CompositeHandler) OnMessage(msg interface{}) error {
	if h.streamer != nil {
		return h.streamer.OnMessage(msg)
	}
	return nil
}

// OnError implements MessageHandler.
// Forwards to the MessageStreamer if set.
func (h *CompositeHandler) OnError(err error) {
	if h.streamer != nil {
		h.streamer.OnError(err)
	}
}

// OnComplete implements MessageHandler.
// Forwards to the CompletionCallback if set.
func (h *CompositeHandler) OnComplete(result *CompletionResult) {
	if h.completer != nil {
		h.completer.OnComplete(result)
	}
}

// OnApproval implements MessageHandler.
// Forwards to the ApprovalHandler if set, otherwise auto-approves.
func (h *CompositeHandler) OnApproval(ctx context.Context, req PermissionRequest) (PermissionResult, error) {
	if h.approval != nil {
		return h.approval.OnApproval(ctx, req)
	}
	// Default: auto-approve
	return PermissionResult{Approved: true}, nil
}

// OnAsk implements MessageHandler.
// Forwards to the AskHandler if set, otherwise auto-approves.
func (h *CompositeHandler) OnAsk(ctx context.Context, req AskRequest) (AskResult, error) {
	if h.ask != nil {
		return h.ask.OnAsk(ctx, req)
	}
	// Default: auto-approve
	return AskResult{ID: req.ID, Approved: true}, nil
}

// Ensure CompositeHandler implements MessageHandler
var _ MessageHandler = (*CompositeHandler)(nil)
