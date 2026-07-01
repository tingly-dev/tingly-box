package server

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// nonstreamResponsesToAnthropicBeta handles non-streaming Responses API request
func (s *Server) nonstreamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}

	// Get rule from context for affinity
	var rule *typ.Rule
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	response, cancel, err = forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)

	// Update affinity entry with message ID
	s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	nonstream.WriteAnthropicMessage(c, anthropicResp)

}

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (s *Server) streamResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, streamRec)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicBetaStream(hc, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// streamResponsesToAnthropicBeta handles streaming Responses API request
func (s *Server) assembleResponsesToAnthropicBeta(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, streamRec)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicBetaAssembly(c, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
func (s *Server) nonstreamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatResp, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIChatToResponsesNonStream(hc, chatResp, reqCtx.RequestModel)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
// Extracted from openai_responses.go:202-216
func (s *Server) streamOpenAIChatToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, string(chatReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to create streaming request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIChatToResponsesStream(hc, chatStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (non-streaming).
func (s *Server) nonstreamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()
	wrapper := s.clientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	anthropicResp, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleAnthropicBetaToResponsesNonStream(hc, anthropicResp, reqCtx.RequestModel)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
}

// streamAnthropicBetaToResponses handles a Responses-shaped client
// request that has been normalized to Anthropic Beta and forwarded to an
// Anthropic provider (streaming).
func (s *Server) streamAnthropicBetaToResponses(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	responseModel := reqCtx.ResponseModel
	anthropicReq := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	ctx := c.Request.Context()

	wrapper := s.clientPool.GetAnthropicClient(ctx, provider, string(anthropicReq.Model))
	fc := forwarding.NewForwardContext(ctx, provider)
	anthropicStream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to create streaming request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleAnthropicBetaToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}

func (s *Server) streamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesStream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, recorder)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(responsesStream)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, recorder)
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleResponsesToOpenAIChatStream(hc, primedStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
	if recorder != nil {
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

func (s *Server) nonstreamResponsesToChat(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, recorder *ProtocolRecorder) {
	actualModel := reqCtx.RequestModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	responsesResp, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		SendErrorResponse(c, err, "Failed to forward request")
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleResponsesToOpenAIChatNonStream(hc, responsesResp)
	s.trackUsageWithTokenUsage(c, tokenUsage, nil)
	if recorder != nil {
		recorder.SetAssembledResponse(nonstream.OpenAIResponsesToChat(responsesResp, reqCtx.ResponseModel))
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

// nonstreamResponsesToAnthropic handles non-streaming Responses API request for v1
// This converts Anthropic v1 request directly to Responses API format, calls the API, and converts back to v1
func (s *Server) nonstreamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get protocol recorder if exists
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}

	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	// Use standard OpenAI Responses API (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	response, cancel, err = forwarding.ForwardOpenAIResponses(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		stream.SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usagepkg.FromOpenAIResponses(response.Usage), nil)

	// Convert Responses API response back to Anthropic v1 format
	anthropicResp := nonstream.ConvertResponsesToAnthropicV1Response(response, proxyModel)

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
func (s *Server) streamResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
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
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	hc := protocol.NewHandleContext(c, proxyModel)
	usage, err := stream.HandleResponsesToAnthropicV1Stream(hc, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}

// streamResponsesToAnthropic handles streaming Responses API request
func (s *Server) assembleResponsesToAnthropic(c *gin.Context, proxyModel string, actualModel string, provider *typ.Provider, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ProtocolRecorder
	if r, exists := c.Get(recorderContextKey); exists {
		recorder = r.(*ProtocolRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	// For standard OpenAI providers, use the OpenAI SDK (session ID already in c.Request.Context)
	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, responsesReq.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, responsesReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.handlePreStreamFailure(c, err, streamRec)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(streamResp)
	if primeErr != nil {
		s.handlePreStreamFailure(c, primeErr, streamRec)
		return
	}

	usage, err := stream.HandleResponsesToAnthropicV1Assembly(c, primedStream, proxyModel)

	// Track usage from stream handler
	if err != nil {
		s.trackUsageWithTokenUsage(c, usage, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	s.trackUsageWithTokenUsage(c, usage, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, usage)
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
