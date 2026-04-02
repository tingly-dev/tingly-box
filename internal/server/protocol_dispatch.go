package server

import (
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

		// TODO: anthropic <-> anthropic beta
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
			recorder.RecordResponse(provider, actualModel)
		}
		c.JSON(http.StatusOK, anthropicResp)
	}
}

func (s *Server) dispatchChainFromAnthropicBeta(
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
			recorder.RecordResponse(provider, actualModel)
		}
		c.JSON(http.StatusOK, anthropicResp)
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
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
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
			s.handleAnthropicV1ViaResponsesAPIStreaming(c, responseModel, actualModel, provider, *req)
		} else if provider.APIBase == protocol.CodexAPIBase {
			s.handleAnthropicV1ViaResponsesAPIAssembly(c, responseModel, actualModel, provider, *req)
		} else {
			s.handleAnthropicV1ViaResponsesAPINonStreaming(c, responseModel, actualModel, provider, *req)
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
		}
	} else {
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
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		case protocol.TypeAnthropicBeta:
			anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, responseModel)
			s.updateAffinityMessageID(c, rule, anthropicResp.ID)
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	}
}
