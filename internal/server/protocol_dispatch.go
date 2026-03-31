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

func (s *Server) dispatchAnthropicBetaChainResult(
	c *gin.Context, reqCtx *transform.TransformContext,
	rule *typ.Rule, provider *typ.Provider,
	isStreaming bool, recorder *ProtocolRecorder,
	actualModel, proxyModel string,
) {
	apiStyle := provider.APIStyle
	switch reqCtx.TargetAPI {
	case protocol.APIAnthropicBeta:
		// Get final transformed request
		req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

		// Use direct Anthropic SDK call
		wrapper := s.clientPool.GetAnthropicClient(provider, actualModel)
		fc := NewForwardContext(c.Request.Context(), provider)

		if isStreaming {
			// Handle streaming request with request context for proper cancellation
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
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1Beta(c, req, streamResp, proxyModel, provider, recorder)
		} else {
			// Handle non-streaming request
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

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			cacheTokens := int(anthropicResp.Usage.CacheReadInputTokens)
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, usage, nil)

			// Update affinity entry with message ID
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)

			session := s.guardrailsSessionFromContext(c, actualModel, provider)
			messageHistory := serverguardrails.MessagesFromAnthropicV1Beta(req.System, req.Messages)
			blocked := s.applyGuardrailsToAnthropicV1BetaNonStreamResponse(c, session, messageHistory, anthropicResp)
			if !blocked {
				s.restoreGuardrailsCredentialAliasesV1BetaResponse(c, anthropicResp)
			}

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
		return

	case protocol.APIGoogle:
		// Get final transformed request
		req := reqCtx.Request.(*protocol.GoogleRequest)

		// Convert Anthropic beta request to Google format
		model, googleReq, cfg := actualModel, req.Content, req.Config

		if isStreaming {
			// Create streaming request with request context for proper cancellation
			wrapper := s.clientPool.GetGoogleClient(provider, model)
			fc := NewForwardContext(c.Request.Context(), provider)
			streamResp, cancel, err := ForwardGoogleStream(fc, wrapper, model, googleReq, cfg)
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

			// Handle the streaming response
			usage, err := stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				s.trackUsageWithTokenUsage(c, usage, err)
				stream.SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageWithTokenUsage(c, usage, nil)

		} else {
			// Handle non-streaming request
			wrapper := s.clientPool.GetGoogleClient(provider, model)
			fc := NewForwardContext(nil, provider)
			resp, _, err := ForwardGoogle(fc, wrapper, model, googleReq, cfg)
			if err != nil {
				stream.SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Convert Google response to Anthropic beta format
			anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(resp, proxyModel)

			// Track usage from response
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

			// Update affinity entry with message ID
			s.updateAffinityMessageID(c, rule, string(anthropicResp.ID))

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}

	case protocol.APIOpenAIResponses:
		// Use Responses API path with Transform Chain
		// Set the rule and provider in context so middleware can use the same rule
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

		if isStreaming {
			s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, proxyModel, actualModel, provider, *req)
		} else {
			if provider.APIBase == protocol.CodexAPIBase {
				s.handleAnthropicV1BetaViaResponsesAPIAssembly(c, proxyModel, actualModel, provider, *req)
			} else {
				s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, proxyModel, actualModel, provider, *req)
			}
		}
		return

	case protocol.APIOpenAIChat:

		// Set the rule and provider in context so middleware can use the same rule
		if rule != nil {
			c.Set("rule", rule)
		}

		// Set provider UUID in context
		c.Set("provider", provider.UUID)
		c.Set("model", actualModel)

		req := reqCtx.Request.(*openai.ChatCompletionNewParams)

		// Clean up temporary fields (e.g., x_thinking)
		request.CleanupOpenaiFields(req)

		// Use OpenAI Chat Completions path
		if isStreaming {
			// Set up stream recorder
			streamRec := newStreamRecorder(recorder)
			if streamRec != nil {
				streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
			}

			// Create streaming request with request context for proper cancellation
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

			// Handle the streaming response
			usage, err := stream.HandleOpenAIToAnthropicBetaStream(c, req, streamResp, proxyModel)
			if err != nil {
				s.trackUsageWithTokenUsage(c, usage, err)
				stream.SendInternalError(c, err.Error())
				if streamRec != nil {
					streamRec.RecordError(err)
				}
				return
			}

			// Track usage from stream handler
			s.trackUsageWithTokenUsage(c, usage, nil)

			// Finish recording and assemble response
			if streamRec != nil {
				streamRec.Finish(proxyModel, usage.InputTokens, usage.OutputTokens)
				streamRec.RecordResponse(provider, actualModel)
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
			// Convert OpenAI response back to Anthropic beta format
			anthropicResp := nonstream.ConvertOpenAIToAnthropicBetaResponse(resp, proxyModel)

			// Track usage from response
			inputTokens := int(resp.Usage.PromptTokens)
			outputTokens := int(resp.Usage.CompletionTokens)
			cacheTokens := int(resp.Usage.PromptTokensDetails.CachedTokens)
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens)
			s.trackUsageWithTokenUsage(c, usage, nil)

			// Update affinity entry with message ID
			s.updateAffinityMessageID(c, rule, anthropicResp.ID)

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", apiStyle))
		}
	}
}
