package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// HandleContext provides dependencies for handle functions.
// It uses the builder pattern for optional configuration and hooks.
type HandleContext struct {
	// Gin context
	GinContext *gin.Context

	// Model info
	ResponseModel string

	// Guardrails runtime state shared across request/response/stream phases for
	// one proxied conversation.
	Guardrails *HandleGuardrails

	// Hooks for stream processing (chainable - multiple hooks can be added)
	OnStreamEventHooks    []func(event interface{}) error
	OnStreamCompleteHooks []func()
	OnStreamErrorHooks    []func(err error)

	// Stream configuration flags
	DisableStreamUsage bool // Don't include usage in streaming chunks

	// EstimatedInputTokens is a pre-computed input-token estimate used only as a
	// fallback when the upstream stream reports no usage. The caller computes it
	// and sets it here, so the stream handler depends on this scalar rather than
	// the request.
	EstimatedInputTokens int
}

// NewHandleContext creates a new HandleContext with required dependencies.
func NewHandleContext(c *gin.Context, responseModel string) *HandleContext {
	return &HandleContext{
		GinContext:    c,
		ResponseModel: responseModel,
	}
}

type HandleGuardrails struct {
	Enabled bool

	CredentialMask *guardrailscore.CredentialMaskState
	Stream         *GuardrailsStreamState
}

type GuardrailsStreamState struct {
	// PendingBlockMessages stores early hook verdicts keyed by tool_use id.
	PendingBlockMessages map[string]string
	// PendingBlockedIndex tracks which content block index is currently blocked.
	PendingBlockedIndex map[int]string
	// RewroteBlockedToolUse is set once the current message's tool_use block has
	// been replaced by a synthetic guardrails text block. The subsequent
	// message_delta stop_reason must be rewritten away from tool_use.
	RewroteBlockedToolUse bool
	// AnthropicToolEvents buffers one tool_use block from start -> delta -> stop
	// so the rewrite layer can either flush the original events or replace them.
	AnthropicToolEvents map[int][]GuardrailsBufferedEvent
	// AnthropicToolIDs links the buffered block index back to the provider tool id.
	AnthropicToolIDs map[int]string
}

func (hc *HandleContext) EnsureGuardrails() *HandleGuardrails {
	if hc.Guardrails == nil {
		hc.Guardrails = &HandleGuardrails{}
	}
	return hc.Guardrails
}

func (hc *HandleContext) EnsureGuardrailsStream() *GuardrailsStreamState {
	guardrails := hc.EnsureGuardrails()
	if guardrails.Stream == nil {
		guardrails.Stream = &GuardrailsStreamState{
			PendingBlockMessages: make(map[string]string),
			PendingBlockedIndex:  make(map[int]string),
			AnthropicToolEvents:  make(map[int][]GuardrailsBufferedEvent),
			AnthropicToolIDs:     make(map[int]string),
		}
	}
	return guardrails.Stream
}

type GuardrailsBufferedEvent struct {
	EventType string
	Payload   map[string]interface{}
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
// Internally delegates loop infrastructure to RunLoop.
//
// nextFunc should return (true, nil, event) to continue, (false, nil, nil) to stop,
// or (false, err, nil) on error.
// handleFunc is called for each event after OnStreamEventHooks are invoked.
func (hc *HandleContext) ProcessStream(nextFunc func() (bool, error, interface{}), handleFunc func(interface{}) error) error {
	defer hc.ReleaseStreamState()

	c := hc.GinContext

	if _, ok := c.Writer.(http.Flusher); !ok {
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

	RunLoop(c, func(_ io.Writer) bool {
		// RunLoop only watches clientGone; also check request context here.
		select {
		case <-c.Request.Context().Done():
			return false
		default:
		}

		cont, err, event := nextFunc()
		if err != nil {
			processErr = err
			return false
		}
		if !cont {
			return false
		}

		for _, hook := range hc.OnStreamEventHooks {
			if hookErr := hook(event); hookErr != nil {
				processErr = hookErr
				return false
			}
		}

		if handleFunc != nil {
			if handleErr := handleFunc(event); handleErr != nil {
				processErr = handleErr
				return false
			}
		}

		return true
	})

	if processErr != nil {
		for _, hook := range hc.OnStreamErrorHooks {
			hook(processErr)
		}
		return processErr
	}

	for _, hook := range hc.OnStreamCompleteHooks {
		hook()
	}

	return nil
}

// DispatchStreamError calls all OnStreamError hooks.

// ReleaseStreamState drops per-stream hooks and guardrail buffers after a stream
// has completed or errored. Hook closures often capture assemblers/recorders that
// aggregate response chunks; clearing them prevents protocol stream response
// state from staying reachable through a HandleContext after request completion.
func (hc *HandleContext) ReleaseStreamState() {
	if hc == nil {
		return
	}
	hc.OnStreamEventHooks = nil
	hc.OnStreamCompleteHooks = nil
	hc.OnStreamErrorHooks = nil
	if hc.Guardrails != nil {
		hc.Guardrails.Stream = nil
	}
}

func (hc *HandleContext) DispatchStreamError(err error) {
	for _, hook := range hc.OnStreamErrorHooks {
		hook(err)
	}
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

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// IsContextCanceled checks if the error is due to context cancellation.
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}
