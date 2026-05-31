package protocol

import (
	"context"
	"errors"
	"fmt"
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

	// OnStreamRawEventHooks is the byte-path equivalent of OnStreamEventHooks
	// for stream paths that operate on raw SSE bytes instead of typed events
	// (currently: OpenAI Responses via StreamLoop). Each hook receives the
	// event type and the raw JSON bytes and returns the (possibly modified)
	// bytes; returning a non-nil error aborts the stream.
	OnStreamRawEventHooks []func(eventType string, eventRaw []byte) ([]byte, error)

	// OnNonStreamResponseHooks fires once for non-streaming responses, just
	// before c.JSON writes the body. Each hook may mutate resp in place
	// (typically the same typed struct or map the handler is about to send).
	// Companion of OnStreamRawEventHooks for the non-stream half; used by
	// output injectors to prepend content to the first text slot.
	OnNonStreamResponseHooks []func(resp any)

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

// WithOnStreamRawEvent adds a hook for the raw-bytes stream path. The hook
// receives the event type and the raw JSON bytes and returns the (possibly
// modified) bytes; multiple hooks chain and are applied in order.
//
// Mutation paths use this chain rather than OnStreamEventHooks because most
// passthrough handlers send evt.RawJSON() / pre-marshaled chunk bytes — so
// changes to typed event struct fields would not reach the wire. Use
// WithOnStreamEvent for read-only side effects (recording, accumulation).
func (hc *HandleContext) WithOnStreamRawEvent(hook func(eventType string, eventRaw []byte) ([]byte, error)) *HandleContext {
	hc.OnStreamRawEventHooks = append(hc.OnStreamRawEventHooks, hook)
	return hc
}

// WithOnNonStreamResponse adds a hook fired once per non-streaming response
// just before c.JSON writes the body. Hooks may mutate resp in place.
func (hc *HandleContext) WithOnNonStreamResponse(hook func(resp any)) *HandleContext {
	hc.OnNonStreamResponseHooks = append(hc.OnNonStreamResponseHooks, hook)
	return hc
}

// RunNonStreamResponseHooks calls every registered non-stream response hook
// in order, threading the same resp through each. Safe to call when no hooks
// are registered.
func (hc *HandleContext) RunNonStreamResponseHooks(resp any) {
	for _, hook := range hc.OnNonStreamResponseHooks {
		hook(resp)
	}
}

// RunStreamRawEventHooks applies the registered raw-event hooks in order,
// threading the (possibly modified) bytes through each. Safe to call when
// no hooks are registered — returns the input unchanged with no allocation.
// Returns the first error a hook raised, in which case eventRaw is the value
// going INTO the failed hook (so callers may decide to send-as-is or abort).
func (hc *HandleContext) RunStreamRawEventHooks(eventType string, eventRaw []byte) ([]byte, error) {
	for _, hook := range hc.OnStreamRawEventHooks {
		modified, err := hook(eventType, eventRaw)
		if err != nil {
			return eventRaw, err
		}
		eventRaw = modified
	}
	return eventRaw, nil
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

	// Manual stream loop instead of gin's c.Stream: gin flushes after every
	// step, and its Flush() calls WriteHeaderNow() — so a step that produced
	// nothing (e.g. the stream failed before the first event) would lock in a
	// 200, blocking the handler's post-loop error path from setting a
	// retryable 5xx. We flush only after an event was actually handled.
	flusher := c.Writer.(http.Flusher)
	clientGone := c.Writer.CloseNotify()
streamLoop:
	for {
		// Check cancellation (client disconnect or request context).
		select {
		case <-clientGone:
			break streamLoop
		case <-c.Request.Context().Done():
			break streamLoop
		default:
		}

		// Get next event
		cont, err, event := nextFunc()
		if err != nil {
			processErr = err
			break streamLoop
		}
		if !cont {
			break streamLoop
		}

		// First real chunk: signal the failover gate (when one is wrapping
		// c.Writer) to flush buffered output and switch to pass-through.
		// Opportunistic assert keeps protocol free of any server dependency.
		if cm, ok := c.Writer.(interface{ CommitFirstChunk() }); ok {
			cm.CommitFirstChunk()
		}

		// Call OnStreamEvent hooks first
		for _, hook := range hc.OnStreamEventHooks {
			if hookErr := hook(event); hookErr != nil {
				processErr = hookErr
				break streamLoop
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
				break streamLoop
			}
		}

		flusher.Flush()
	}

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
