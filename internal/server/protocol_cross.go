package server

import (
	"context"

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

// preStreamErrorRecorder and handlePreStreamFailure now live in
// failover_dispatch.go (moved there in Step 9) — this file's calls below use
// that canonical definition directly, no local copy needed.

// nonstreamResponsesToAnthropicBeta handles non-streaming Responses API request
func (ph *ProtocolHandler) nonstreamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	recorder, _ := recording.GetRecorderFromContext(c)

	// Get rule from context for affinity
	var rule *typ.Rule
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	response, cancel, err = forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		ph.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	ph.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	anthropicResp := nonstream.HandleResponsesToAnthropicBeta(response, proxyModel)

	// Update affinity entry with message ID
	ph.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	nonstream.WriteAnthropicMessage(c, anthropicResp)

}

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (ph *ProtocolHandler) streamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicBetaStream(hc, primedStream, proxyModel)

	// Track usage from stream handler
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

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (ph *ProtocolHandler) assembleResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicBetaAssembly(c, primedStream, proxyModel)

	// Track usage from stream handler
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

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
func (ph *ProtocolHandler) nonstreamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatResp, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIChatToResponses(hc, chatResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
// Extracted from openai_responses.go:202-216
func (ph *ProtocolHandler) streamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to create streaming request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIChatToResponsesStream(hc, chatStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (non-streaming).
func (ph *ProtocolHandler) nonstreamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()
	wrapper := ph.deps.ClientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	anthropicResp, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleAnthropicBetaToResponses(hc, anthropicResp, reqCtx.RequestModel)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (streaming).
func (ph *ProtocolHandler) streamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
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
		ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to create streaming request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	ph.trackUsageWithTokenUsage(c, usage, err)
}

func (ph *ProtocolHandler) streamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
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

func (ph *ProtocolHandler) nonstreamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *recording.ProtocolRecorder) {
	actualModel := reqCtx.RequestModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesResp, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
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

// nonstreamResponsesToAnthropic handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (ph *ProtocolHandler) nonstreamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	recorder, _ := recording.GetRecorderFromContext(c)

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	response, cancel, err = forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		ph.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	ph.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	// Convert Responses API response back to Anthropic v1 format
	anthropicResp := nonstream.HandleResponsesToAnthropicV1(response, proxyModel)

	// TODO: require anthropic <-> anthropic beta
	//if ShouldRoundtripResponse(c, "openai") {
	//	roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(&anthropicResp, proxyModel, provider, actualModel)
	//	if err != nil {
	//		stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
	//		return
	//	}
	//	anthropicResp = *roundtripped
	//}

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	nonstream.WriteAnthropicMessage(c, anthropicResp)
}

// streamResponsesToAnthropic handles streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and streams back in v1 format
func (ph *ProtocolHandler) streamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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
		ph.trackUsageFromContext(c, 0, 0, err)
		stream.SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
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

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicV1Stream(hc, primedStream, proxyModel)

	// Track usage from stream handler
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

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// streamResponsesToAnthropic handles streaming Responses API request
func (ph *ProtocolHandler) assembleResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
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

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicV1Assembly(c, primedStream, proxyModel)

	// Track usage from stream handler
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

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
