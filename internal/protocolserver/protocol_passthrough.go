package protocolserver

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// nonstreamOpenAIChat handles non-streaming chat completion requests with MCP runtime support.
func (ph *ProtocolHandler) nonstreamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, stripUsage bool) {
	recorder := recording.FromGin(c)
	req := originalReq

	// Forward request to provider
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	response, _, err := forwarding.ForwardOpenAIChat(fc, wrapper, req)
	if err != nil {
		ph.failForward(c, err)
		return
	}

	// Extract usage from response
	inputTokens := int(response.Usage.PromptTokens)
	outputTokens := int(response.Usage.CompletionTokens)
	cacheTokens := int(response.Usage.PromptTokensDetails.CachedTokens)

	// Track usage
	usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Update response model if configured
	responseMap["model"] = responseModel
	if stripUsage {
		delete(responseMap, "usage")
	}

	if ShouldRoundtripResponse(c, "anthropic") {
		roundtripped, err := RoundtripOpenAIResponseViaAnthropic(response, responseModel, provider, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to roundtrip response: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}
		responseMap = roundtripped
		responseMap["model"] = responseModel
		if stripUsage {
			delete(responseMap, "usage")
		}
	}

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
	if recorder != nil {
		recorder.SetAssembledResponse(responseMap)
		recorder.RecordResponse(provider, req.Model)
	}
}

// streamOpenAIChat handles streaming chat completion requests.
func (ph *ProtocolHandler) streamOpenAIChat(c *gin.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, responseModel string, disableStreamUsage bool) {
	recorder := recording.FromGin(c)
	req := originalReq

	// Estimate input tokens up front and hand the scalar to the stream handler,
	// so it depends on the estimate rather than the request for the usage fallback.
	estimatedInputTokens := token.EstimateInputTokensSimple(req)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	// Create handle context and handle stream
	hc := protocol.NewHandleContext(c, responseModel)
	hc.DisableStreamUsage = disableStreamUsage
	hc.EstimatedInputTokens = estimatedInputTokens

	usage, err := stream.HandleOpenAIChatStream(hc, streamResp)

	// Track usage from stream handler
	ph.trackUsageWithTokenUsage(c, usage, err)

	// Emit the record for the passthrough stream. No chunk tap exists on this
	// path (the SSE bytes are relayed by HandleOpenAIChatStream), so the final
	// response is synthesized from the writer state; the request-side capture
	// points are unaffected.
	if recorder != nil {
		if err != nil {
			recorder.RecordError(err)
		} else {
			recorder.EnableStreaming()
			recorder.RecordResponse(provider, req.Model)
		}
	}
}

// nonstreamOpenAIResponses handles Responses API passthrough (non-streaming)
func (ph *ProtocolHandler) nonstreamOpenAIResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	recorder := recording.FromGin(c)
	params := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, string(params.Model))
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	response, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failRequest(c, err, "Failed to forward request")
		return
	}

	hc := protocol.NewHandleContext(c, reqCtx.ResponseModel)
	tokenUsage, _ := nonstream.HandleOpenAIResponses(hc, response)
	ph.trackUsageWithTokenUsage(c, tokenUsage, nil)
	if recorder != nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
}

// streamOpenAIResponses handles Responses API passthrough (streaming)
// Moved from openai_responses.go:421-456
func (ph *ProtocolHandler) streamOpenAIResponses(c *gin.Context, reqCtx *transform.TransformContext, provider *typ.Provider) {
	recorder := recording.FromGin(c)
	responseModel := reqCtx.ResponseModel
	params := reqCtx.Request.(*responses.ResponseNewParams)

	// Create streaming request with request context for proper cancellation
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, params.Model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	respStream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.handlePreStreamFailure(c, err, recorder)
		return
	}

	primedStream, primeErr := stream.PrimeResponsesStream(respStream)
	if primeErr != nil {
		ph.handlePreStreamFailure(c, primeErr, recorder)
		return
	}

	// Handle the streaming response
	hc := protocol.NewHandleContext(c, responseModel)
	usage, err := stream.HandleOpenAIResponsesStream(hc, primedStream, responseModel)

	// Track usage from stream handler
	ph.trackUsageWithTokenUsage(c, usage, err)
}
