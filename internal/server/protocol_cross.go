package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/server/recording"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file hosts the Responses↔Anthropic / Responses↔Chat cross-format
// paths. The Responses→Anthropic surface is a 2×3 matrix — {v1, beta} ×
// {nonstream, stream, assemble} — whose six entry points share two cores:
// forwardResponsesNonstream and forwardResponsesStream. Only the final
// format-conversion step differs per cell, injected as a closure.
//
// The remaining paths below (Responses→Chat, Chat→Responses,
// AnthropicBeta→Responses) deliberately keep their own bodies: they carry
// different recording semantics (the dispatch-level ProtocolRecorder rather
// than a per-call StreamRecorder, no assembled-response capture on stream
// paths) and their response writing is owned by the protocol/stream handlers,
// so folding them into the cores would change observable recording behavior.

// forwardResponsesNonstream forwards a Responses API request upstream
// (non-streaming) and converts the response back to an Anthropic shape via
// convert, which returns the response to write plus its message ID for
// session-affinity bookkeeping.
func (ph *ProtocolHandler) forwardResponsesNonstream(
	c *gin.Context, actualModel string, provider *typ.Provider,
	responsesReq responses.ResponseNewParams,
	convert func(*responses.Response) (resp any, messageID string),
) {
	// Get protocol recorder if exists
	recorder, _ := recording.GetRecorderFromContext(c)

	// Get rule from context for affinity
	var rule *typ.Rule
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
	}

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	response, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failForward(c, recorder, err)
		return
	}

	ph.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	anthropicResp, messageID := convert(response)

	// Update affinity entry with message ID
	if rule != nil {
		ph.updateAffinityMessageID(c, rule, messageID)
	}

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	nonstream.WriteAnthropicMessage(c, anthropicResp)
}

// forwardResponsesStream forwards a Responses API request upstream
// (streaming), primes the stream so lazy upstream errors surface before any
// byte hits the wire (letting failover retry), and hands the primed stream to
// handle for source-format conversion. handle owns writing the client
// response; usage tracking and stream recording are handled here.
func (ph *ProtocolHandler) forwardResponsesStream(
	c *gin.Context, proxyModel, actualModel string, provider *typ.Provider,
	responsesReq responses.ResponseNewParams,
	handle func(c *gin.Context, primed stream.ResponsesStreamIter) (*protocol.TokenUsage, error),
) {
	// Get scenario recorder and set up stream recorder
	recorder, _ := recording.GetRecorderFromContext(c)
	streamRec := recording.NewStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.handlePreStreamFailure(c, err, streamRec)
		return
	}

	// Prime the stream: SDK streams are lazy, real upstream errors only
	// surface on first Next(). Forcing it here lets failover retry
	// before any byte hits the wire.
	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := handle(c, primedStream)
	if err != nil {
		ph.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}
}

// nonstreamResponsesToAnthropic handles a non-streaming Anthropic v1 request
// forwarded via the Responses API.
func (ph *ProtocolHandler) nonstreamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesNonstream(c, actualModel, provider, responsesReq,
		func(rs *responses.Response) (any, string) {
			msg := nonstream.HandleResponsesToAnthropicV1(rs, proxyModel)
			return msg, string(msg.ID)
		})
}

// nonstreamResponsesToAnthropicBeta handles a non-streaming Anthropic beta
// request forwarded via the Responses API.
func (ph *ProtocolHandler) nonstreamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesNonstream(c, actualModel, provider, responsesReq,
		func(rs *responses.Response) (any, string) {
			msg := nonstream.HandleResponsesToAnthropicBeta(rs, proxyModel)
			return msg, string(msg.ID)
		})
}

// streamResponsesToAnthropic streams a Responses API upstream back to an
// Anthropic v1 client as SSE.
func (ph *ProtocolHandler) streamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesStream(c, proxyModel, actualModel, provider, responsesReq,
		func(c *gin.Context, primed stream.ResponsesStreamIter) (*protocol.TokenUsage, error) {
			hc := protocol.NewHandleContext(c, proxyModel)
			return stream.HandleResponsesToAnthropicV1Stream(hc, primed, proxyModel)
		})
}

// streamResponsesToAnthropicBeta streams a Responses API upstream back to an
// Anthropic beta client as SSE.
func (ph *ProtocolHandler) streamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesStream(c, proxyModel, actualModel, provider, responsesReq,
		func(c *gin.Context, primed stream.ResponsesStreamIter) (*protocol.TokenUsage, error) {
			hc := protocol.NewHandleContext(c, proxyModel)
			return stream.HandleResponsesToAnthropicBetaStream(hc, primed, proxyModel)
		})
}

// assembleResponsesToAnthropic consumes a Responses API upstream stream and
// assembles it into a single non-streaming Anthropic v1 response (used for
// providers, e.g. Codex, that only stream).
func (ph *ProtocolHandler) assembleResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesStream(c, proxyModel, actualModel, provider, responsesReq,
		func(c *gin.Context, primed stream.ResponsesStreamIter) (*protocol.TokenUsage, error) {
			return stream.HandleResponsesToAnthropicV1Assembly(c, primed, proxyModel)
		})
}

// assembleResponsesToAnthropicBeta consumes a Responses API upstream stream
// and assembles it into a single non-streaming Anthropic beta response.
func (ph *ProtocolHandler) assembleResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	ph.forwardResponsesStream(c, proxyModel, actualModel, provider, responsesReq,
		func(c *gin.Context, primed stream.ResponsesStreamIter) (*protocol.TokenUsage, error) {
			return stream.HandleResponsesToAnthropicBetaAssembly(c, primed, proxyModel)
		})
}

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
func (ph *ProtocolHandler) nonstreamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatResp, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		ph.failRequest(c, recorder, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIChatToResponses(hc, chatResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
func (ph *ProtocolHandler) streamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, recorder, err, "Failed to create streaming request")
		return
	}
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIChatToResponsesStream(hc, chatStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (non-streaming).
func (ph *ProtocolHandler) nonstreamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()
	wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	anthropicResp, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, recorder, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleAnthropicBetaToResponses(hc, anthropicResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (streaming).
func (ph *ProtocolHandler) streamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(ctx, provider)
	anthropicStream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, recorder, err, "Failed to create streaming request")
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

func (ph *ProtocolHandler) streamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesStream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(responsesStream)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, recorder)
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleResponsesToOpenAIChatStream(hc, primedStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
	if recorder != nil {
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

func (ph *ProtocolHandler) nonstreamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	actualModel := reqCtx.RequestModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesResp, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, recorder, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	chatResp, tokenUsage, _ := nonstream.HandleResponsesToOpenAIChat(hc, responsesResp)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
	if recorder != nil {
		recorder.SetAssembledResponse(chatResp)
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}
