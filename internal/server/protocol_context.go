package server

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ForwardContext provides dependencies for forward functions.
// It uses the builder pattern for optional configuration and hooks.
type ForwardContext struct {
	// Required dependencies
	Provider *typ.Provider
	BaseCtx  context.Context // Base context (e.g., request context for cancellation support)

	// Optional configuration
	Timeout time.Duration

	// Hooks (chainable - multiple hooks can be added)
	BeforeRequestHooks []func(ctx context.Context, req interface{}) (context.Context, error)
	AfterRequestHooks  []func(ctx context.Context, resp interface{}, err error)
}

// NewForwardContext creates a new ForwardContext with required dependencies.
// The timeout is set to the provider's default timeout.
// baseCtx is the base context for the request:
//   - Use context.Background() for non-streaming requests
//   - Use c.Request.Context() for streaming requests to support client cancellation
func NewForwardContext(baseCtx context.Context, provider *typ.Provider) *ForwardContext {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &ForwardContext{
		Provider: provider,
		BaseCtx:  baseCtx,
		Timeout:  time.Duration(provider.Timeout) * time.Second,
	}
}

// WithTimeout sets the timeout for the request.
// If not set, the provider's default timeout is used.
func (fc *ForwardContext) WithTimeout(timeout time.Duration) *ForwardContext {
	fc.Timeout = timeout
	return fc
}

// WithBeforeRequest adds a hook that is called before the request is sent.
// Multiple hooks can be added and will be called in order.
// Each hook can modify the context and return an error to abort the request.
func (fc *ForwardContext) WithBeforeRequest(hook func(context.Context, interface{}) (context.Context, error)) *ForwardContext {
	fc.BeforeRequestHooks = append(fc.BeforeRequestHooks, hook)
	return fc
}

// WithAfterRequest adds a hook that is called after the request completes.
// Multiple hooks can be added and will be called in order.
// Each hook receives the response and any error that occurred.
func (fc *ForwardContext) WithAfterRequest(hook func(context.Context, interface{}, error)) *ForwardContext {
	fc.AfterRequestHooks = append(fc.AfterRequestHooks, hook)
	return fc
}

// PrepareContext prepares the final context for the request.
// It applies the BeforeRequest hooks and adds the scenario to the context.
// It also sets up the timeout and returns a cancel function.
// If BaseCtx is not set, it uses context.Background() as the base.
//
// The order of operations matches the original implementation:
// 1. Apply BeforeRequest hooks
// 2. Add timeout
func (fc *ForwardContext) PrepareContext(req interface{}) (context.Context, context.CancelFunc, error) {
	ctx := fc.BaseCtx
	if ctx == nil {
		ctx = context.Background()
	}

	// Apply BeforeRequest hooks in order
	for _, hook := range fc.BeforeRequestHooks {
		var err error
		ctx, err = hook(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("BeforeRequest hook failed: %w", err)
		}
	}

	// Add timeout FIRST (matching old code order for stream)
	ctx, cancel := context.WithTimeout(ctx, fc.Timeout)

	return ctx, cancel, nil
}

// Complete calls all AfterRequest hooks (if set) with the response and error.
// Hooks are called in the order they were added.
// This should be called after the request completes, regardless of success or failure.
func (fc *ForwardContext) Complete(ctx context.Context, resp interface{}, err error) {
	for _, hook := range fc.AfterRequestHooks {
		hook(ctx, resp, err)
	}
}
