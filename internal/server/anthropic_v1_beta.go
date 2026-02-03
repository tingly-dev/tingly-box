package server

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// anthropicMessagesV1Beta implements beta messages API
func (s *Server) anthropicMessagesV1Beta(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule) {
	actualModel := selectedService.Model

	// Extract scenario from URL path for recording
	scenario := c.Param("scenario")

	// Get scenario recorder if exists (set by AnthropicMessages)
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	// Check if streaming is requested
	isStreaming := req.Stream

	req.Model = anthropic.Model(actualModel)

	// Set the rule and provider in context so middleware can use the same rule
	if rule != nil {
		c.Set("rule", rule)
	}

	// Ensure max_tokens is set (Anthropic API requires this)
	// and cap it at the model's maximum allowed value
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {

	} else {
		if req.MaxTokens == 0 {
			req.MaxTokens = int64(s.config.GetDefaultMaxTokens())
		}
		// Cap max_tokens at the model's maximum to prevent API errors
		maxAllowed := s.templateManager.GetMaxTokensForModel(provider.Name, actualModel)
		if req.MaxTokens > int64(maxAllowed) {
			req.MaxTokens = int64(maxAllowed)
		}
	}

	// Set provider UUID in context (Service.Provider uses UUID, not name)
	c.Set("provider", provider.UUID)
	c.Set("model", selectedService.Model)

	// Check provider's API style to decide which path to take
	apiStyle := provider.APIStyle

	switch apiStyle {
	case protocol.APIStyleAnthropic:
		// Use direct Anthropic SDK call
		if isStreaming {
			// Handle streaming request
			streamResp, err := s.forwardAnthropicStreamRequestV1Beta(provider, req.BetaMessageNewParams, scenario)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_creation_failed")
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}
			// Handle the streaming response
			s.handleAnthropicStreamResponseV1Beta(c, req.BetaMessageNewParams, streamResp, proxyModel, actualModel, rule, provider, recorder)
		} else {
			// Handle non-streaming request
			anthropicResp, err := s.forwardAnthropicRequestV1Beta(provider, req.BetaMessageNewParams, scenario)
			if err != nil {
				s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "forward_failed")
				SendForwardingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Track usage from response
			inputTokens := int(anthropicResp.Usage.InputTokens)
			outputTokens := int(anthropicResp.Usage.OutputTokens)
			s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

			// FIXME: now we use req model as resp model
			anthropicResp.Model = anthropic.Model(proxyModel)

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
		return
	case protocol.APIStyleGoogle:

		// Convert Anthropic beta request to Google format
		model, googleReq, cfg := request.ConvertAnthropicBetaToGoogleRequest(&req.BetaMessageNewParams, 0)

		if isStreaming {
			// Create streaming request
			streamResp, err := s.forwardGoogleStreamRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendStreamingError(c, err)
				if recorder != nil {
					recorder.RecordError(err)
				}
				return
			}

			// Handle the streaming response
			err = stream.HandleGoogleToAnthropicBetaStreamResponse(c, streamResp, proxyModel)
			if err != nil {
				SendInternalError(c, err.Error())
				if recorder != nil {
					recorder.RecordError(err)
				}
			}

			// Track usage from stream (would be accumulated in handler)
			// For Google, usage is tracked in the stream handler

		} else {
			// Handle non-streaming request
			resp, err := s.forwardGoogleRequest(provider, model, googleReq, cfg)
			if err != nil {
				SendForwardingError(c, err)
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
			if resp.UsageMetadata != nil {
				inputTokens = int(resp.UsageMetadata.PromptTokenCount)
				outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			}
			s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

			// Record response if scenario recording is enabled
			if recorder != nil {
				recorder.SetAssembledResponse(anthropicResp)
				recorder.RecordResponse(provider, actualModel)
			}
			c.JSON(http.StatusOK, anthropicResp)
		}
	case protocol.APIStyleOpenAI:
		// Check if model prefers Responses API (for models like Codex)
		// This is used for ChatGPT backend API which only supports Responses API
		useResponsesAPI := selectedService.PreferCompletions()

		// Also check the probe cache if not already determined
		if !useResponsesAPI {
			preferredEndpoint := s.GetPreferredEndpointForModel(provider, actualModel)
			useResponsesAPI = preferredEndpoint == "responses"
		}

		if useResponsesAPI {
			// Use Responses API path (for Codex and other models that prefer it)
			// Convert Anthropic beta request to Responses API format
			responsesReq := request.ConvertAnthropicBetaToResponsesRequestWithProvider(&req.BetaMessageNewParams, provider, actualModel)

			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			if isStreaming {
				s.handleAnthropicV1BetaViaResponsesAPIStreaming(c, req, proxyModel, actualModel, provider, selectedService, rule, responsesReq)
			} else {
				s.handleAnthropicV1BetaViaResponsesAPINonStreaming(c, req, proxyModel, actualModel, provider, selectedService, rule, responsesReq)
			}
		} else {
			// Use Chat Completions path (fallback)
			openaiReq := request.ConvertAnthropicBetaToOpenAIRequestWithProvider(&req.BetaMessageNewParams, true, provider, actualModel)

			// Set the rule and provider in context so middleware can use the same rule
			if rule != nil {
				c.Set("rule", rule)
			}

			// Set provider UUID in context
			c.Set("provider", provider.UUID)
			c.Set("model", actualModel)

			// Use OpenAI Chat Completions path
			if isStreaming {
				// Set up stream recorder
				streamRec := newStreamRecorder(recorder)
				if streamRec != nil {
					streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
				}

				// Create streaming request
				streamResp, err := s.forwardOpenAIStreamRequest(provider, openaiReq)
				if err != nil {
					SendStreamingError(c, err)
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Handle the streaming response
				err = stream.HandleOpenAIToAnthropicV1BetaStreamResponse(c, openaiReq, streamResp, proxyModel)
				if err != nil {
					SendInternalError(c, err.Error())
					if streamRec != nil {
						streamRec.RecordError(err)
					}
					return
				}

				// Finish recording and assemble response
				if streamRec != nil {
					streamRec.Finish(proxyModel, 0, 0) // Usage is tracked internally
					streamRec.RecordResponse(provider, actualModel)
				}

			} else {
				resp, err := s.forwardOpenAIRequest(provider, openaiReq)
				if err != nil {
					SendForwardingError(c, err)
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
				s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

				// Record response if scenario recording is enabled
				if recorder != nil {
					recorder.SetAssembledResponse(anthropicResp)
					recorder.RecordResponse(provider, actualModel)
				}
				c.JSON(http.StatusOK, anthropicResp)
			}
		}
	default:
		c.JSON(http.StatusBadRequest, "tingly-box: invalid api style")
		if recorder != nil {
			recorder.RecordError(fmt.Errorf("invalid api style: %s", apiStyle))
		}
	}
}

// forwardAnthropicRequestV1Beta forwards request using Anthropic SDK with proper types (beta)
func (s *Server) forwardAnthropicRequestV1Beta(provider *typ.Provider, req anthropic.BetaMessageNewParams, scenario string) (*anthropic.BetaMessage, error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	// Create context with scenario for recording
	ctx := context.WithValue(context.Background(), client.ScenarioContextKey, scenario)

	// Make the request using Anthropic SDK with timeout (provider.Timeout is in seconds)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	message, err := wrapper.BetaMessagesNew(ctx, req)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// forwardAnthropicStreamRequestV1Beta forwards streaming request using Anthropic SDK (beta)
func (s *Server) forwardAnthropicStreamRequestV1Beta(provider *typ.Provider, req anthropic.BetaMessageNewParams, scenario string) (*anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], error) {
	// Get or create Anthropic client wrapper from pool
	wrapper := s.clientPool.GetAnthropicClient(provider, string(req.Model))

	logrus.Debugln("Creating Anthropic beta streaming request")

	// Create context with scenario for recording
	ctx := context.WithValue(context.Background(), client.ScenarioContextKey, scenario)

	// Use background context for streaming
	// The stream will manage its own lifecycle and timeout
	// We don't use a timeout here because streaming responses can take longer
	streamResp := wrapper.BetaMessagesNewStreaming(ctx, req)

	return streamResp, nil
}

// handleAnthropicStreamResponseV1Beta processes the Anthropic beta streaming response and sends it to the client
func (s *Server) handleAnthropicStreamResponseV1Beta(c *gin.Context, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion], respModel, actualModel string, rule *typ.Rule, provider *typ.Provider, recorder *ScenarioRecorder) {
	// Accumulate usage from stream
	var inputTokens, outputTokens int
	var hasUsage bool

	// Create stream recorder for unified recording
	streamRec := newStreamRecorder(recorder)

	// Set SSE headers
	SetupSSEHeaders(c)

	// Check SSE support
	if !CheckSSESupport(c) {
		return
	}

	// Use gin.Stream for cleaner streaming handling
	c.Stream(func(w io.Writer) bool {
		if !streamResp.Next() {
			return false
		}

		event := streamResp.Current()
		event.Message.Model = anthropic.Model(respModel)

		// Record event using streamRecorder
		streamRec.RecordV1BetaEvent(&event)

		// Accumulate usage from message_stop event
		if event.Usage.InputTokens > 0 {
			inputTokens = int(event.Usage.InputTokens)
			hasUsage = true
		}
		if event.Usage.OutputTokens > 0 {
			outputTokens = int(event.Usage.OutputTokens)
			hasUsage = true
		}

		// Convert the event to JSON and send as SSE
		c.SSEvent(event.Type, event)
		return true
	})

	// Finish recording and assemble response
	streamRec.Finish(respModel, inputTokens, outputTokens)

	// Check for stream errors
	if err := streamResp.Err(); err != nil {
		// Track usage with error status
		if hasUsage {
			s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "error", "stream_error")
		}
		MarshalAndSendErrorEvent(c, err.Error(), "stream_error", "stream_failed")
		// Record error
		streamRec.RecordError(err)
		return
	}

	// Track successful streaming completion
	if hasUsage {
		s.trackUsage(c, rule, provider, actualModel, respModel, inputTokens, outputTokens, true, "success", "")
	}

	// Send completion event
	SendFinishEvent(c)

	// Record the response after stream completes
	streamRec.RecordResponse(provider, actualModel)
}

// forwardGoogleRequest forwards request to Google API
func (s *Server) forwardGoogleRequest(provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// Get or create Google client wrapper from pool
	wrapper := s.clientPool.GetGoogleClient(provider, model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
	}

	// Make the request with timeout
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	response, err := wrapper.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// forwardGoogleStreamRequest forwards streaming request to Google API
func (s *Server) forwardGoogleStreamRequest(provider *typ.Provider, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	// Get or create Google client wrapper from pool
	wrapper := s.clientPool.GetGoogleClient(provider, model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
	}

	logrus.Debugln("Creating Google streaming request")

	// Use background context for streaming
	ctx := context.Background()
	streamResp := wrapper.GenerateContentStream(ctx, model, contents, config)

	return streamResp, nil
}

// handleAnthropicV1BetaViaChatCompletions handles Anthropic v1beta request using OpenAI Chat Completions API
func (s *Server) handleAnthropicV1BetaViaChatCompletions(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, isStreaming bool) {

}

// handleAnthropicV1BetaViaResponsesAPINonStreaming handles non-streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPINonStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder if exists
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}

	var response *responses.Response
	var err error

	// Check if this is a ChatGPT backend API provider
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API handler
		response, err = s.forwardChatGPTBackendRequest(provider, responsesReq)
	} else {
		// Use standard OpenAI Responses API
		response, err = s.forwardResponsesRequest(provider, responsesReq)
	}

	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "forward_failed")
		SendForwardingError(c, err)
		if recorder != nil {
			recorder.RecordError(err)
		}
		return
	}

	// Extract usage from response
	inputTokens := int(response.Usage.InputTokens)
	outputTokens := int(response.Usage.OutputTokens)

	// Track usage
	s.trackUsage(c, rule, provider, actualModel, proxyModel, inputTokens, outputTokens, false, "success", "")

	// Convert Responses API response back to Anthropic beta format
	anthropicResp := nonstream.ConvertResponsesToAnthropicBetaResponse(response, proxyModel)

	// Record response if scenario recording is enabled
	if recorder != nil {
		recorder.SetAssembledResponse(anthropicResp)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, anthropicResp)
}

// handleAnthropicV1BetaViaResponsesAPIStreaming handles streaming Responses API request
func (s *Server) handleAnthropicV1BetaViaResponsesAPIStreaming(c *gin.Context, req protocol.AnthropicBetaMessagesRequest, proxyModel string, actualModel string, provider *typ.Provider, selectedService *loadbalance.Service, rule *typ.Rule, responsesReq responses.ResponseNewParams) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}
	// Check if this is a ChatGPT backend API provider
	// These providers need special handling because they use custom HTTP implementation
	if provider.APIBase == protocol.ChatGPTBackendAPIBase {
		// Use the ChatGPT backend API streaming handler
		// This handler sends the stream directly to the client in OpenAI Responses API format
		s.handleChatGPTBackendStreamingRequest(c, provider, responsesReq, proxyModel, actualModel, rule)
		return
	}

	// For standard OpenAI providers, use the OpenAI SDK
	streamResp, cancel, err := s.forwardResponsesStreamRequest(provider, responsesReq)
	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_creation_failed")
		SendStreamingError(c, err)
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}
	defer cancel()

	// Handle the streaming response
	// Use the dedicated stream handler to convert Responses API to Anthropic beta format
	err = stream.HandleResponsesToAnthropicV1BetaStreamResponse(c, streamResp, proxyModel)

	// Track usage from stream (would be accumulated in handler)
	// For now, we'll track minimal usage since the handler manages it
	if err != nil {
		s.trackUsage(c, rule, provider, actualModel, proxyModel, 0, 0, false, "error", "stream_error")
		if streamRec != nil {
			streamRec.RecordError(err)
		}
		return
	}

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(proxyModel, 0, 0) // Usage is tracked internally
		streamRec.RecordResponse(provider, actualModel)
	}

	// Success - usage tracking is handled inside the stream handler
	// Note: The handler tracks usage when response.completed event is received
}
