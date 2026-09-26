package protocolserver

import (
	"fmt"

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

// stageAnthropicAttempt is one provider attempt of an Anthropic client
// (V1 or Beta) served by a Protocol Stage pipeline.
type stageAnthropicAttempt struct {
	// Client is the client's protocol: anthropic_v1 or anthropic_beta.
	Client protocol.APIType
	// Request is the prepared Beta request (a V1 request upgraded at the edge).
	Request *anthropic.BetaMessageNewParams
	Rule    *typ.Rule
	// Provider and ActualModel identify the upstream; ResponseModel is the
	// model the client asked for and sees in the response.
	Provider      *typ.Provider
	ActualModel   string
	ResponseModel string
	Streaming     bool
}

// serveStageAnthropic is the HTTP adapter for Anthropic clients: it runs the
// pipeline and writes its answer through the same writers, tracking, affinity
// and recording calls as the legacy Anthropic passthrough, so the client sees
// the same bytes. A V1 client gets its Beta answer downgraded at this edge.
func (ph *ProtocolHandler) serveStageAnthropic(c *gin.Context, endpoint stage.Endpoint, attempt stageAnthropicAttempt) {
	if attempt.Client != protocol.TypeAnthropicV1 && attempt.Client != protocol.TypeAnthropicBeta {
		ph.failRequest(c, fmt.Errorf("stage adapter: %q is not an Anthropic client protocol", attempt.Client), "Unsupported client protocol")
		return
	}
	call := stage.Call{Request: attempt.Request}
	if attempt.Streaming {
		ph.streamStageAnthropic(c, endpoint, call, attempt)
		return
	}
	ph.completeStageAnthropic(c, endpoint, call, attempt)
}

func (ph *ProtocolHandler) completeStageAnthropic(c *gin.Context, endpoint stage.Endpoint, call stage.Call, attempt stageAnthropicAttempt) {
	recorder := recording.FromGin(c)
	response, err := endpoint.Complete(c.Request.Context(), call)
	if err != nil {
		ph.failForward(c, err)
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
	ph.updateAffinityMessageID(c, attempt.Rule, message.ID)
	message.Model = anthropic.Model(attempt.ResponseModel)

	if recorder != nil {
		recorder.SetAssembledResponse(message)
		recorder.RecordResponse(attempt.Provider, attempt.ActualModel)
	}
	if attempt.Client == protocol.TypeAnthropicBeta {
		nonstream.WriteAnthropicMessage(c, message)
		return
	}
	v1, err := sdkstream.AnthropicV1Downgrade(message)
	if err != nil {
		ph.failRequest(c, err, "Failed to downgrade response for an Anthropic v1 client")
		return
	}
	v1.Model = anthropic.Model(attempt.ResponseModel)
	nonstream.WriteAnthropicMessage(c, v1)
}

func (ph *ProtocolHandler) streamStageAnthropic(c *gin.Context, endpoint stage.Endpoint, call stage.Call, attempt stageAnthropicAttempt) {
	recorder := recording.FromGin(c)
	ctx := c.Request.Context()
	events, err := endpoint.Stream(ctx, call)
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	hc := protocol.NewHandleContext(c, attempt.ResponseModel)
	recording.AttachRecorderHooks(hc, recorder, attempt.ActualModel, attempt.Provider)

	var usage *protocol.TokenUsage
	if attempt.Client == protocol.TypeAnthropicBeta {
		usage, err = stream.HandleAnthropicBeta(hc, sdkstream.AnthropicBeta(ctx, events))
	} else {
		usage, err = stream.HandleAnthropic(hc, sdkstream.AnthropicV1(ctx, events))
	}
	// Usage covers every round of the pipeline, not only the client-visible
	// message_start and final message_delta.
	if total := events.Result().Usage; total != nil && total.HasUsage() {
		usage = total
	}
	ph.trackUsageWithTokenUsage(c, usage, err)
	holdAfterSideEffects(c, err)
}

// holdAfterSideEffects commits the failover gate when err was raised after
// the pipeline ran a server tool: another service would run it again, so the
// error goes to the client instead of triggering failover.
func holdAfterSideEffects(c *gin.Context, err error) {
	if stage.HasCommittedSideEffects(err) {
		CommitFirstChunkIfGate(c.Writer)
	}
}
