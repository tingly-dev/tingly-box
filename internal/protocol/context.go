package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
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
	OnStreamEventHooks     []func(event interface{}) error
	OnStreamCompleteHooks  []func()
	OnStreamErrorHooks     []func(err error)
	OnStreamAssembledHooks []func(*anthropic.Message)

	// streamAssembler accumulates Anthropic stream events into a final
	// message. Created lazily by WithOnStreamAssembled; nil disables assembly.
	streamAssembler *assembler.AnthropicStreamAssembler

	// Stream configuration flags
	DisableStreamUsage bool // Don't include usage in streaming chunks
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

// WithOnStreamAssembled adds a hook that receives the final assembled message
// once an Anthropic stream completes successfully. Registering a hook enables
// stream assembly: ProcessStream feeds every v1/v1beta event into an internal
// assembler and invokes the hooks with the result before OnStreamComplete.
func (hc *HandleContext) WithOnStreamAssembled(hook func(*anthropic.Message)) *HandleContext {
	if hc.streamAssembler == nil {
		hc.streamAssembler = assembler.NewAnthropicStreamAssembler()
	}
	hc.OnStreamAssembledHooks = append(hc.OnStreamAssembledHooks, hook)
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

		// Feed the event into the stream assembler when assembly is enabled.
		if hc.streamAssembler != nil {
			switch evt := event.(type) {
			case *anthropic.MessageStreamEventUnion:
				hc.streamAssembler.RecordV1Event(evt)
			case *anthropic.BetaRawMessageStreamEventUnion:
				hc.streamAssembler.RecordV1BetaEvent(evt)
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

	// Deliver the assembled message before completion hooks run, so
	// consumers can store it ahead of any finalisation.
	if hc.streamAssembler != nil && len(hc.OnStreamAssembledHooks) > 0 {
		assembled := hc.streamAssembler.Finish(hc.ResponseModel, 0, 0)
		for _, hook := range hc.OnStreamAssembledHooks {
			hook(assembled)
		}
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
