package protocolserver

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/recording"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file hosts the Responses↔Chat and AnthropicBeta→Responses
// cross-format paths. Anthropic clients on a Responses provider are served by
// the Stage pipeline (serveAnthropicOnOpenAI).

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
func (ph *ProtocolHandler) nonstreamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatResp, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		ph.failRequest(c, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIChatToResponses(hc, chatResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
func (ph *ProtocolHandler) streamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, err, "Failed to create streaming request")
		return
	}
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIChatToResponsesStream(hc, chatStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (non-streaming).
func (ph *ProtocolHandler) nonstreamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()
	wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	anthropicResp, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleAnthropicBetaToResponses(hc, anthropicResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (streaming).
func (ph *ProtocolHandler) streamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
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
		ph.failRequest(c, err, "Failed to create streaming request")
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

func (ph *ProtocolHandler) streamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	recorder := recording.FromGin(c)
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

func (ph *ProtocolHandler) nonstreamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	recorder := recording.FromGin(c)
	actualModel := reqCtx.RequestModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesResp, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, err, "Failed to forward request")
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
