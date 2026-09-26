package protocolserver

import (
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/sdkstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// StageOpenAIAttempt is one provider attempt of an OpenAI client (Chat or
// Responses) served by a Protocol Stage pipeline whose working protocol is
// Anthropic Beta.
type StageOpenAIAttempt struct {
	// Client is the client's protocol: openai_chat or openai_responses.
	Client protocol.APIType
	// Request is the client's request converted to Beta at the edge.
	Request *anthropic.BetaMessageNewParams
	// Provider and ActualModel identify the upstream; ResponseModel is the
	// model the client asked for and sees in the response.
	Provider      *typ.Provider
	ActualModel   string
	ResponseModel string
	Streaming     bool
	// DisableStreamUsage leaves usage out of a Chat stream; StripUsage leaves
	// it out of a complete Chat answer.
	DisableStreamUsage bool
	StripUsage         bool
}

// ServeStageOpenAI is the HTTP adapter for OpenAI clients on a Beta
// pipeline: it runs the pipeline and converts its Beta answer at this edge
// through the same writers as the legacy Anthropic-provider paths, so the
// client sees the same bytes.
func (ph *ProtocolHandler) ServeStageOpenAI(c *gin.Context, endpoint stage.Endpoint, attempt StageOpenAIAttempt) {
	if attempt.Client != protocol.TypeOpenAIChat && attempt.Client != protocol.TypeOpenAIResponses {
		ph.failRequest(c, fmt.Errorf("stage adapter: %q is not an OpenAI client protocol", attempt.Client), "Unsupported client protocol")
		return
	}
	call := stage.Call{Request: attempt.Request}
	if attempt.Streaming {
		ph.streamStageOpenAI(c, endpoint, call, attempt)
		return
	}
	ph.completeStageOpenAI(c, endpoint, call, attempt)
}

func (ph *ProtocolHandler) completeStageOpenAI(c *gin.Context, endpoint stage.Endpoint, call stage.Call, attempt StageOpenAIAttempt) {
	recorder := recording.FromGin(c)
	response, err := endpoint.Complete(c.Request.Context(), call)
	if err != nil {
		desc := "Failed to forward request"
		if attempt.Client == protocol.TypeOpenAIChat {
			desc = "Failed to forward Anthropic request"
		}
		ph.failRequest(c, err, desc)
		holdAfterSideEffects(c, err)
		return
	}
	message, ok := response.Value.(*anthropic.BetaMessage)
	if !ok || message == nil {
		ph.failRequest(c, fmt.Errorf("stage adapter: response has type %T, want *anthropic.BetaMessage", response.Value), "Invalid pipeline response")
		return
	}

	// Usage covers every round of the pipeline, not only the final message.
	usage := response.Usage
	if usage == nil {
		usage = usagepkg.FromAnthropicBetaMessage(message.Usage)
	}
	ph.trackUsageWithTokenUsage(c, usage, nil)

	if attempt.Client == protocol.TypeOpenAIResponses {
		hc := protocol.NewHandleContext(c, attempt.ResponseModel)
		_, _ = nonstream.HandleAnthropicBetaToResponses(hc, message, attempt.ActualModel)
		return
	}

	openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(message, attempt.ResponseModel, attempt.Provider, attempt.ActualModel)
	if ShouldRoundtripResponse(c, "anthropic") {
		roundtripped, err := RoundtripOpenAIMapViaAnthropic(openaiResp, attempt.ResponseModel, attempt.Provider, attempt.ActualModel)
		if err != nil {
			SendErrorResponse(c, err, "Failed to roundtrip response")
			return
		}
		openaiResp = roundtripped
	}
	if attempt.StripUsage {
		delete(openaiResp, "usage")
	}
	if recorder != nil {
		recorder.SetAssembledResponse(message)
		recorder.RecordResponse(attempt.Provider, attempt.ActualModel)
	}
	c.JSON(http.StatusOK, openaiResp)
}

func (ph *ProtocolHandler) streamStageOpenAI(c *gin.Context, endpoint stage.Endpoint, call stage.Call, attempt StageOpenAIAttempt) {
	recorder := recording.FromGin(c)
	ctx := c.Request.Context()
	events, err := endpoint.Stream(ctx, call)
	if err != nil {
		ph.failRequest(c, err, "Failed to create streaming request")
		return
	}

	hc := protocol.NewHandleContext(c, attempt.ResponseModel)
	// As for Anthropic clients (streamStageAnthropic): an SSE comment keeps
	// the started stream alive while the pipeline runs a server tool.
	keepAlive := sdkstream.OnHeartbeat(func() {
		if c.Writer.Written() {
			_, _ = fmt.Fprint(c.Writer, ": keep-alive\n\n")
			c.Writer.Flush()
		}
	})
	betaEvents := sdkstream.AnthropicBeta(ctx, events, keepAlive)

	var usage *protocol.TokenUsage
	if attempt.Client == protocol.TypeOpenAIResponses {
		usage, err = stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, betaEvents, attempt.ResponseModel)
	} else {
		usage, err = stream.AnthropicToOpenAIStream(hc, attempt.Request, betaEvents, attempt.ResponseModel, attempt.DisableStreamUsage)
	}
	// Usage covers every round of the pipeline.
	if total := events.Result().Usage; total != nil && total.HasUsage() {
		usage = total
	}
	ph.trackUsageWithTokenUsage(c, usage, err)
	if err != nil && attempt.Client == protocol.TypeOpenAIChat {
		// As the legacy Chat path: a stream that failed before its first
		// byte is answered with an error body (the writer already wrote one
		// when it could; this call only registers the error for logging).
		SendErrorResponse(c, err, "Failed to create streaming request")
		if recorder != nil {
			recorder.RecordError(err)
		}
	}
	holdAfterSideEffects(c, err)
}
