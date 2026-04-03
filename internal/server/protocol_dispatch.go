// Package server
// since we do refactoring and migrating step by step, some api names are not unified, this will be updated in future
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) dispatchChainResult(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	switch reqCtx.TargetAPI {
	case protocol.TypeAnthropicV1:
		s.dispatchChainFromAnthropicV1(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeAnthropicBeta:
		s.dispatchChainFromAnthropicBeta(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeGoogle:
		s.dispatchChainFromGoogle(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeOpenAIResponses:
		s.dispatchChainFromResponses(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeOpenAIChat:
		s.dispatchChainFromOpenAIChat(c, reqCtx, rule, provider, isStreaming, recorder)
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", provider.APIStyle))
		}
	}
}

// ── Anthropic direct ────────────────────────────────────────────────────

func (s *Server) dispatchChainFromAnthropicV1(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIResponses:
		if isStreaming {
			s.streamAnthropicV1ToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		} else {
			s.nonstreamAnthropicV1ToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		}
	default:
		s.dispatchAnthropicToAnthropicV1(c, reqCtx, rule, provider, isStreaming, recorder)
	}
}

// dispatchAnthropicToAnthropicV1 handles Anthropic→Anthropic v1 passthrough (original behavior)
func (s *Server) dispatchAnthropicToAnthropicV1(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.MessageNewParams)

	wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)
	fc := NewForwardContext(c.Request.Context(), provider)

	if isStreaming {
		streamResp, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageFromContext(c, 0, 0, err)
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		hc := protocol.NewHandleContext(c, responseModel)

		// Record TTFT when the first streaming chunk arrives
		firstTokenRecorded := false
		hc.WithOnStreamEvent(func(_ interface{}) error {
			if !firstTokenRecorded {
				SetFirstTokenTime(c)
				firstTokenRecorded = true
			}
			return nil
		})

		// Add recorder hooks if recorder is available
		if recorder != nil {
			onEvent, onComplete, onError := NewRecorderHooksWithModel(recorder, actualModel, provider)
			if onEvent != nil {
				hc.WithOnStreamEvent(onEvent)
			}
			if onComplete != nil {
				hc.WithOnStreamComplete(onComplete)
			}
			if onError != nil {
				hc.WithOnStreamError(onError)
			}
		}

		// Anthropic v1 only adapts request history; the shared runtime owns all
		// enablement checks and hook wiring after this point.
		session := s.guardrailsSessionFromContext(c, actualModel, provider)
		s.attachGuardrailsHooks(c, hc, session, serverguardrails.MessagesFromAnthropicV1(req.System, req.Messages))

		usageStat, err := stream.HandleAnthropicV1Stream(hc, *req, streamResp)
		s.trackUsageWithTokenUsage(c, usageStat, err)
	} else {
		anthropicResp, cancel, err := ForwardAnthropicV1(fc, wrapper, req)
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

		inputTokens := int(anthropicResp.Usage.InputTokens)
		outputTokens := int(anthropicResp.Usage.OutputTokens)
		cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		s.trackUsageWithTokenUsage(c, usage, nil)

		s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

		// FIXME: now we use req model as resp model
		anthropicResp.Model = anthropic.Model(responseModel)

		// TODO: disable this round trip until we support anthropic <-> anthropic beta
		//if ShouldRoundtripResponse(c, "openai") {
		//	roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
		//	if err != nil {
		//		stream.SendInternalError(c, "Failed to roundtrip response: "+err.Error())
		//		return
		//	}
		//	anthropicResp = roundtripped
		//}

		session := s.guardrailsSessionFromContext(c, actualModel, provider)
		messageHistory := serverguardrails.MessagesFromAnthropicV1(req.System, req.Messages)
		blocked := s.applyGuardrailsToAnthropicV1NonStreamResponse(c, session, messageHistory, anthropicResp)
		if !blocked {
			s.restoreGuardrailsCredentialAliasesV1Response(c, anthropicResp)
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		c.JSON(http.StatusOK, anthropicResp)
	}
}

// dispatchOpenAIChatFromAnthropicV1 handles OpenAI→Anthropic v1 conversion.
// The client expects OpenAI format, so responses are converted back.
func (s *Server) dispatchOpenAIChatFromAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)
	fc := NewForwardContext(c.Request.Context(), provider)

	if isStreaming {
		streamResp, cancel, err := ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageFromContext(c, 0, 0, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to create streaming request: " + err.Error(),
					Type:    "api_error",
				},
			})
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		disableStreamUsage := false
		if v, ok := reqCtx.Extra["cursor_compat"]; ok {
			disableStreamUsage = v.(bool)
		}
		if reqCtx.ScenarioFlags != nil {
			disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.DisableStreamUsage
		}

		inputTokens, outputTokens, err := stream.AnthropicToOpenAIStream(c, req, streamResp, responseModel, disableStreamUsage)
		if err != nil {
			if inputTokens > 0 || outputTokens > 0 {
				tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
				s.trackUsageWithTokenUsage(c, tokenUsage, err)
			}
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to create streaming request: " + err.Error(),
					Type:    "api_error",
				},
			})
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		if inputTokens > 0 || outputTokens > 0 {
			tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
			s.trackUsageWithTokenUsage(c, tokenUsage, nil)
		}
	} else {
		anthropicResp, cancel, err := ForwardAnthropicV1Beta(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageFromContext(c, 0, 0, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to forward Anthropic request: " + err.Error(),
					Type:    "api_error",
				},
			})
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		inputTokens := int(anthropicResp.Usage.InputTokens)
		outputTokens := int(anthropicResp.Usage.OutputTokens)
		cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens + anthropicResp.Usage.CacheCreationInputTokens)
		tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		s.trackUsageWithTokenUsage(c, tokenUsage, nil)

		openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
		if ShouldRoundtripResponse(c, "anthropic") {
			roundtripped, err := RoundtripOpenAIMapViaAnthropic(openaiResp, responseModel, provider, actualModel)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "Failed to roundtrip response: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			openaiResp = roundtripped
		}
		cursorCompat := false
		if v, ok := reqCtx.Extra["cursor_compat"]; ok {
			cursorCompat = v.(bool)
		}
		if cursorCompat {
			delete(openaiResp, "usage")
		}

		if recorder != nil {
			recorder.SetAssembledResponse(anthropicResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		c.JSON(http.StatusOK, openaiResp)
	}
}

func (s *Server) dispatchChainFromAnthropicBeta(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	switch reqCtx.SourceAPI {
	case protocol.TypeOpenAIChat:
		s.dispatchOpenAIChatFromAnthropicBeta(c, reqCtx, rule, provider, isStreaming, recorder)
	default:
		actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
		req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

		wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)
		fc := NewForwardContext(c.Request.Context(), provider)

		if isStreaming {
			streamResp, cancel, err := ForwardAnthropicV1BetaStream(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				s.trackUsageFromContext(c, 0, 0, err)
				stream.SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			s.handleAnthropicStreamResponseV1Beta(c, req, streamResp, responseModel, provider, recorder)
		} else {
			anthropicResp, cancel, err := ForwardAnthropicV1Beta(fc, wrapper, req)
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

			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, usage, nil)

			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(responseModel)

			session := s.guardrailsSessionFromContext(c, actualModel, provider)
			messageHistory := serverguardrails.MessagesFromAnthropicV1Beta(req.System, req.Messages)
			blocked := s.applyGuardrailsToAnthropicV1BetaNonStreamResponse(c, session, messageHistory, anthropicResp)
			if !blocked {
				s.restoreGuardrailsCredentialAliasesV1BetaResponse(c, anthropicResp)
			}

			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// ── Google ──────────────────────────────────────────────────────────────

func (s *Server) dispatchChainFromGoogle(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	googleReq := reqCtx.Request.(*protocol.GoogleRequest)
	model, req, cfg := actualModel, googleReq.Contents, googleReq.Config

	if isStreaming {
		wrapper := s.clientPool.GetGoogleClient(provider, model)
		fc := NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := ForwardGoogleStream(fc, wrapper, model, req, cfg)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		var usage *protocol.TokenUsage
		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			usage, err = stream.HandleGoogleToAnthropicStreamResponse(c, streamResp, responseModel)
		case protocol.TypeAnthropicBeta:
			usage, err = stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, responseModel)
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
	} else {
		wrapper := s.clientPool.GetGoogleClient(provider, model)
		fc := NewForwardContext(nil, provider)
		resp, _, err := ForwardGoogle(fc, wrapper, model, req, cfg)
		if err != nil {
			stream.SendForwardingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		inputTokens := 0
		outputTokens := 0
		cacheTokens := 0
		if resp.UsageMetadata != nil {
			inputTokens = int(resp.UsageMetadata.PromptTokenCount)
			outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			cacheTokens = int(resp.UsageMetadata.CachedContentTokenCount)
		}
		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		s.trackUsageWithTokenUsage(c, usage, nil)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.ConvertGoogleToAnthropicResponse(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// ── OpenAI Responses API ────────────────────────────────────────────────

func (s *Server) dispatchChainFromResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel

	if rule != nil {
		c.Set("rule", rule)
	}

	// Set provider UUID in context
	c.Set("provider", provider.UUID)
	c.Set("model", actualModel)

	// Set context flag to indicate original request was v1 format
	// The ChatGPT backend streaming handler will use this to send responses in v1 format
	c.Set("original_request_format", "v1")

	logrus.Debugf("[Anthropic Beta] Using Transform Chain for Responses API for model=%s", actualModel)

	req := reqCtx.Request.(*responses.ResponseNewParams)

	switch reqCtx.SourceAPI {
	case protocol.TypeAnthropicV1:
		logrus.Debugf("[AnthropicV1] Using Transform Chain for Responses API for model=%s", actualModel)
		if isStreaming {
			s.streamAnthropicV1ToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		} else if provider.APIBase == protocol.CodexAPIBase {
			s.handleAnthropicV1ViaResponsesAPIAssembly(c, responseModel, actualModel, provider, *req)
		} else {
			s.nonstreamAnthropicV1ToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		}
	case protocol.TypeAnthropicBeta:
		logrus.Debugf("[Anthropic Beta] Using Transform Chain for Responses API for model=%s", actualModel)
		if isStreaming {
			s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, responseModel, actualModel, provider, *req)
		} else if provider.APIBase == protocol.CodexAPIBase {
			s.handleAnthropicV1BetaViaResponsesAPIAssembly(c, responseModel, actualModel, provider, *req)
		} else {
			s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, responseModel, actualModel, provider, *req)
		}
	case protocol.TypeOpenAIChat:
		// Client sent Responses API, but provider needs Chat format
		// Forward as Chat, then convert response back to Responses format
		s.dispatchOpenAIChatFromResponses(c, reqCtx, rule, provider, isStreaming, recorder)
	case protocol.TypeOpenAIResponses:
		// Responses API passthrough
		if isStreaming {
			s.streamOpenAIResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		} else {
			s.nonstreamOpenAIResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		}
	}
}

// ── OpenAI Chat Completions ─────────────────────────────────────────────

func (s *Server) dispatchChainFromOpenAIChat(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel

	if rule != nil {
		c.Set("rule", rule)
	}
	c.Set("provider", provider.UUID)
	c.Set("model", actualModel)

	req := reqCtx.Request.(*openai.ChatCompletionNewParams)
	request.CleanupOpenaiFields(req)

	if isStreaming {
		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardOpenAIChatStream(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				stream.SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			usage, err := stream.HandleOpenAIToAnthropicStreamResponse(c, req, streamResp, responseModel)
			if err != nil {
				s.trackUsageWithTokenUsage(c, usage, err)
				stream.SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			s.trackUsageWithTokenUsage(c, usage, nil)
		case protocol.TypeAnthropicBeta:
			streamRec := newStreamRecorder(recorder)
			if streamRec != nil {
				streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
			}

			wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardOpenAIChatStream(fc, wrapper, req)
			if cancel != nil {
				defer cancel()
			}
			if err != nil {
				stream.SendStreamingError(c, err)
				if streamRec != nil {
					streamRec.RecordError(err)
				}
				return
			}

			usage, err := stream.HandleOpenAIToAnthropicBetaStream(c, req, streamResp, responseModel)
			if err != nil {
				s.trackUsageWithTokenUsage(c, usage, err)
				stream.SendInternalError(c, err.Error())
				if streamRec != nil {
					streamRec.RecordError(err)
				}
				return
			}
			s.trackUsageWithTokenUsage(c, usage, nil)

			if streamRec != nil {
				streamRec.Finish(responseModel, usage.InputTokens, usage.OutputTokens)
				streamRec.RecordResponse(provider, actualModel)
			}
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: source and target are both OpenAI Chat format
			shouldIntercept, _ := reqCtx.Extra["should_intercept"].(bool)
			shouldStripTools, _ := reqCtx.Extra["should_strip_tools"].(bool)

			disableStreamUsage := false
			if v, ok := reqCtx.Extra["cursor_compat"]; ok {
				disableStreamUsage = v.(bool)
			}
			if reqCtx.ScenarioFlags != nil {
				disableStreamUsage = disableStreamUsage || reqCtx.ScenarioFlags.DisableStreamUsage
			}

			s.handleOpenAIChatStreamingRequest(c, provider, req, responseModel, shouldIntercept, shouldStripTools, disableStreamUsage)
		case protocol.TypeOpenAIResponses:
			s.streamOpenAIChatToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
		}
	} else {
		switch reqCtx.SourceAPI {
		case protocol.TypeOpenAIChat:
			// OpenAI passthrough: delegate to handleNonStreamingRequest for tool interceptor support
			shouldIntercept, _ := reqCtx.Extra["should_intercept"].(bool)
			shouldStripTools, _ := reqCtx.Extra["should_strip_tools"].(bool)
			stripUsage := false
			if v, ok := reqCtx.Extra["cursor_compat"]; ok {
				stripUsage = v.(bool)
			}

			s.handleNonStreamingRequest(c, provider, req, responseModel, shouldIntercept, shouldStripTools, stripUsage)
			return
		case protocol.TypeOpenAIResponses:
			s.nonstreamOpenAIChatToResponses(c, reqCtx, rule, provider, isStreaming, recorder)
			return
		default:
			// Forward request to provider for format conversion
		}

		wrapper := s.clientPool.GetOpenAIClient(provider, req.Model)
		fc := NewForwardContext(nil, provider)
		resp, _, err := ForwardOpenAIChat(fc, wrapper, req)
		if err != nil {
			stream.SendForwardingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		inputTokens := int(resp.Usage.PromptTokens)
		outputTokens := int(resp.Usage.CompletionTokens)
		cacheTokens := int(resp.Usage.PromptTokensDetails.CachedTokens)
		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		s.trackUsageWithTokenUsage(c, usage, nil)

		switch reqCtx.SourceAPI {
		case protocol.TypeAnthropicV1:
			anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(resp, responseModel)
			if ShouldRoundtripResponse(c, "openai") {
				roundtripped, err := RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp, responseModel, provider, actualModel)
				if err != nil {
					stream.SendInternalError(c, "Failed to roundtrip resp: "+err.Error())
					return
				}
				anthropicResp = roundtripped
			}
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, anthropicResp.ID)
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}

// dispatchOpenAIChatFromResponses handles Chat→Responses→Chat conversion.
// Client expects Chat format, provider uses Responses format.
func (s *Server) dispatchOpenAIChatFromResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	actualModel, responseModel := reqCtx.RequestModel, reqCtx.ResponseModel
	req := reqCtx.Request.(*responses.ResponseNewParams)

	wrapper := s.clientPool.GetOpenAIClient(provider, actualModel)
	fc := NewForwardContext(c.Request.Context(), provider)

	if isStreaming {
		responsesStream, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, *req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to create streaming request: " + err.Error(),
					Type:    "api_error",
				},
			})
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		hc := protocol.NewHandleContext(c, responseModel)
		firstTokenRecorded := false
		hc.WithOnStreamEvent(func(_ interface{}) error {
			if !firstTokenRecorded {
				SetFirstTokenTime(c)
				firstTokenRecorded = true
			}
			return nil
		})
		usage, err := stream.HandleResponsesToOpenAIChatStream(hc, responsesStream, responseModel)
		s.trackUsageWithTokenUsage(c, usage, err)
		if recorder != nil {
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
	} else {
		responsesResp, cancel, err := ForwardOpenAIResponses(fc, wrapper, *req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to forward request: " + err.Error(),
					Type:    "api_error",
				},
			})
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		inputTokens := int(responsesResp.Usage.InputTokens)
		outputTokens := int(responsesResp.Usage.OutputTokens)
		cacheTokens := int(responsesResp.Usage.InputTokensDetails.CachedTokens)
		tokenUsage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
		s.trackUsageWithTokenUsage(c, tokenUsage, nil)

		chatResp := nonstream.OpenAIResponsesToChat(responsesResp, responseModel)
		if recorder != nil {
			recorder.SetAssembledResponse(chatResp)
			recorder.RecordResponse(provider, reqCtx.RequestModel)
		}
		c.JSON(http.StatusOK, chatResp)
	}
}

// ── OpenAI Responses Handlers ─────────────────────────────────────────────

// nonstreamOpenAIResponses handles Responses API passthrough (non-streaming)
// Moved from openai_responses.go:371-419
func (s *Server) nonstreamOpenAIResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	params := reqCtx.Request.(*responses.ResponseNewParams)

	// Forward request to provider
	var response *responses.Response
	var err error
	var cancel context.CancelFunc

	wrapper := s.clientPool.GetOpenAIClient(provider, string(params.Model))
	fc := NewForwardContext(nil, provider)
	response, cancel, err = ForwardOpenAIResponses(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}

	if err != nil {
		// Track error with no usage
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	// Extract usage from response
	inputTokens := int64(response.Usage.InputTokens)
	outputTokens := int64(response.Usage.OutputTokens)
	cacheTokens := int64(response.Usage.InputTokensDetails.CachedTokens)

	// Track usage
	s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil)

	// Override model in response if needed
	if responseModel != reqCtx.RequestModel {
		// Create a copy of the response with updated model
		responseJSON, _ := json.Marshal(response)
		var responseMap map[string]any
		if err := json.Unmarshal(responseJSON, &responseMap); err == nil {
			responseMap["model"] = responseModel
			if recorder != nil {
				recorder.SetAssembledResponse(response)
				recorder.RecordResponse(provider, reqCtx.RequestModel)
			}
			c.JSON(http.StatusOK, responseMap)
			return
		}
	}

	if recorder != nil {
		recorder.SetAssembledResponse(response)
		recorder.RecordResponse(provider, reqCtx.RequestModel)
	}
	// Return response as-is
	c.JSON(http.StatusOK, response)
}

// streamOpenAIResponses handles Responses API passthrough (streaming)
// Moved from openai_responses.go:421-456
func (s *Server) streamOpenAIResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	params := reqCtx.Request.(*responses.ResponseNewParams)

	// Create streaming request with request context for proper cancellation
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	fc := NewForwardContext(c.Request.Context(), provider)
	respStream, cancel, err := ForwardOpenAIResponsesStream(fc, wrapper, *params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		// Track error with no usage
		s.trackUsageFromContext(c, 0, 0, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	// Handle the streaming response
	hc := protocol.NewHandleContext(c, responseModel)
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})
	usage, err := stream.HandleOpenAIResponsesStream(hc, respStream, responseModel)

	// Track usage from stream handler
	s.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamOpenAIChatToResponses handles Chat → Responses conversion (non-streaming)
// Extracted from openai_responses.go:218-233
func (s *Server) nonstreamOpenAIChatToResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(provider, string(chatReq.Model))
	fc := NewForwardContext(nil, provider)
	chatResp, _, err := ForwardOpenAIChat(fc, wrapper, chatReq)
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Message: "Failed to forward request: " + err.Error(), Type: "api_error"},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}
	inputTokens := chatResp.Usage.PromptTokens
	outputTokens := chatResp.Usage.CompletionTokens
	cacheTokens := chatResp.Usage.PromptTokensDetails.CachedTokens
	s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(inputTokens), int(outputTokens), int(cacheTokens)), nil)
	c.JSON(http.StatusOK, buildResponsesPayloadFromChat(chatResp, responseModel, reqCtx.RequestModel))
}

// streamOpenAIChatToResponses handles Chat → Responses conversion (streaming)
// Extracted from openai_responses.go:202-216
func (s *Server) streamOpenAIChatToResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	chatReq := reqCtx.Request.(*openai.ChatCompletionNewParams)

	wrapper := s.clientPool.GetOpenAIClient(provider, string(chatReq.Model))
	fc := NewForwardContext(c.Request.Context(), provider)
	chatStream, cancel, err := ForwardOpenAIChatStream(fc, wrapper, chatReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Message: "Failed to create streaming request: " + err.Error(), Type: "api_error"},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}
	usage, err := stream.HandleOpenAIChatToResponsesStream(c, chatStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}

// nonstreamAnthropicV1ToResponses handles Anthropic v1 → Responses (non-streaming)
// Extracted from openai_responses.go:265-280
func (s *Server) nonstreamAnthropicV1ToResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	anthropicReq := reqCtx.Request.(*anthropic.MessageNewParams)

	wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
	fc := NewForwardContext(nil, provider)
	anthropicResp, cancel, err := ForwardAnthropicV1(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Message: "Failed to forward request: " + err.Error(), Type: "api_error"},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
	s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(anthropicResp.Usage.InputTokens), int(anthropicResp.Usage.OutputTokens), cacheTokens), nil)
	c.JSON(http.StatusOK, buildResponsesPayloadFromAnthropic(anthropicResp, responseModel, reqCtx.RequestModel))
}

// streamAnthropicV1ToResponses handles Anthropic v1 → Responses (streaming)
// Extracted from openai_responses.go:238-262
func (s *Server) streamAnthropicV1ToResponses(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
) {
	responseModel := reqCtx.ResponseModel
	anthropicReq := reqCtx.Request.(*anthropic.MessageNewParams)

	wrapper := s.clientPool.GetAnthropicClient(provider, string(anthropicReq.Model))
	fc := NewForwardContext(c.Request.Context(), provider)
	anthropicStream, cancel, err := ForwardAnthropicV1Stream(fc, wrapper, anthropicReq)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		s.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(0, 0, 0), err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Message: "Failed to create streaming request: " + err.Error(), Type: "api_error"},
		})
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	hc := protocol.NewHandleContext(c, responseModel)
	firstTokenRecorded := false
	hc.WithOnStreamEvent(func(_ interface{}) error {
		if !firstTokenRecorded {
			SetFirstTokenTime(c)
			firstTokenRecorded = true
		}
		return nil
	})
	usage, err := stream.HandleAnthropicToOpenAIResponsesStream(hc, anthropicStream, responseModel)
	s.trackUsageWithTokenUsage(c, usage, err)
}
