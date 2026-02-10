package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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

// HandleContext provides dependencies for handle functions.
// It uses the builder pattern for optional configuration and hooks.
type HandleContext struct {
	// Gin context
	GinContext *gin.Context

	// Provider info
	Provider      *typ.Provider
	ActualModel   string
	ResponseModel string

	// Hooks for stream processing (chainable - multiple hooks can be added)
	OnStreamEventHooks    []func(event interface{}) error
	OnStreamCompleteHooks []func()
	OnStreamErrorHooks    []func(err error)
}

// NewHandleContext creates a new HandleContext with required dependencies.
func NewHandleContext(c *gin.Context, provider *typ.Provider, actualModel, responseModel string) *HandleContext {
	return &HandleContext{
		GinContext:    c,
		Provider:      provider,
		ActualModel:   actualModel,
		ResponseModel: responseModel,
	}
}

// WithOnStreamEvent adds a hook that is called for each stream event.
// Multiple hooks can be added and will be called in order.
func (hc *HandleContext) WithOnStreamEvent(hook func(interface{}) error) *HandleContext {
	hc.OnStreamEventHooks = append(hc.OnStreamEventHooks, hook)
	return hc
}

// WithOnStreamComplete adds a hook that is called when stream completes successfully.
// Multiple hooks can be added and will be called in order.
func (hc *HandleContext) WithOnStreamComplete(hook func()) *HandleContext {
	hc.OnStreamCompleteHooks = append(hc.OnStreamCompleteHooks, hook)
	return hc
}

// WithOnStreamError adds a hook that is called when stream encounters an error.
// Multiple hooks can be added and will be called in order.
func (hc *HandleContext) WithOnStreamError(hook func(error)) *HandleContext {
	hc.OnStreamErrorHooks = append(hc.OnStreamErrorHooks, hook)
	return hc
}

// SetupSSEHeaders sets the standard SSE (Server-Sent Events) headers.
func (hc *HandleContext) SetupSSEHeaders() {
	c := hc.GinContext
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}

// ProcessStream provides a generic framework for processing streaming responses.
// It handles context cancellation, error checking, and event processing.
//
// nextFunc should return (true, nil, event) to continue, (false, nil, nil) to stop,
// or (false, err, nil) on error.
// handleFunc is called for each event after OnStreamEventHooks are invoked.
// It can be used to send the event to the client.
func (hc *HandleContext) ProcessStream(nextFunc func() (bool, error, interface{}), handleFunc func(interface{}) error) error {
	c := hc.GinContext

	// Check if streaming is supported
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return fmt.Errorf("streaming not supported")
	}

	var processErr error

	// Use gin.Stream for proper streaming handling
	c.Stream(func(w io.Writer) bool {
		// Check context cancellation first
		select {
		case <-c.Request.Context().Done():
			return false
		default:
		}

		// Get next event
		cont, err, event := nextFunc()
		if err != nil {
			processErr = err
			return false
		}
		if !cont {
			return false
		}

		// Call OnStreamEvent hooks first
		for _, hook := range hc.OnStreamEventHooks {
			if hookErr := hook(event); hookErr != nil {
				processErr = hookErr
				return false
			}
		}

		// Call the provided handler function (e.g., to send to client)
		if handleFunc != nil {
			if handleErr := handleFunc(event); handleErr != nil {
				processErr = handleErr
				return false
			}
		}

		return true
	})

	// Call OnStreamError hooks if there was an error
	if processErr != nil {
		for _, hook := range hc.OnStreamErrorHooks {
			hook(processErr)
		}
		return processErr
	}

	// Call OnStreamComplete hooks on success
	for _, hook := range hc.OnStreamCompleteHooks {
		hook()
	}

	return nil
}

// CallOnStreamComplete calls all OnStreamComplete hooks.
// This is useful for non-streaming handlers that still need to invoke complete hooks.
func (hc *HandleContext) CallOnStreamComplete() {
	for _, hook := range hc.OnStreamCompleteHooks {
		hook()
	}
}

// SendError sends an error response to the client.
func (hc *HandleContext) SendError(err error, errorType, code string) {
	c := hc.GinContext

	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: err.Error(),
			Type:    errorType,
			Code:    code,
		},
	})
}
