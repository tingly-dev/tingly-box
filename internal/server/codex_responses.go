package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	streamhandler "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// forwardChatGPTBackendRequest forwards a request to ChatGPT backend API using the correct format
// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
func (s *Server) forwardChatGPTBackendRequest(provider *typ.Provider, params responses.ResponseNewParams) (*responses.Response, error) {
	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		return nil, fmt.Errorf("failed to get OpenAI client")
	}

	logrus.Infof("provider: %s (ChatGPT backend API)", provider.Name)

	// Make HTTP request to ChatGPT backend API
	resp, cancel, err := s.makeChatGPTBackendRequest(wrapper, provider, params)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Errorf("[ChatGPT] API error: %s", string(respBody))
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Read streaming response and accumulate chunks using protocol stream handler
	result, err := streamhandler.AccumulateChatGPTBackendStream(resp.Body)
	if err != nil {
		return nil, err
	}

	return streamhandler.ConvertStreamResultToResponse(result, string(params.Model))
}

// handleChatGPTBackendStreamingRequest handles streaming requests for ChatGPT backend API providers
func (s *Server) handleChatGPTBackendStreamingRequest(c *gin.Context, provider *typ.Provider, params responses.ResponseNewParams, responseModel, actualModel string) {
	// Get scenario recorder and set up stream recorder
	var recorder *ScenarioRecorder
	if r, exists := c.Get("scenario_recorder"); exists {
		recorder = r.(*ScenarioRecorder)
	}
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	wrapper := s.clientPool.GetOpenAIClient(provider, params.Model)
	if wrapper == nil {
		s.trackUsageFromContext(c, 0, 0, fmt.Errorf("no_client"))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to get OpenAI client",
				Type:    "api_error",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		s.trackUsageFromContext(c, 0, 0, fmt.Errorf("streaming_unsupported"))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
			},
		})
		return
	}

	// Make HTTP request to ChatGPT backend API for streaming
	resp, cancel, err := s.makeChatGPTBackendRequest(wrapper, provider, params)
	if err != nil {
		s.trackUsageFromContext(c, 0, 0, err)
		logrus.Errorf("[ChatGPT] Streaming request failed: %v", err)
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": "Failed to create streaming request: " + err.Error(),
				"type":    "api_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}
	defer cancel()
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Errorf("[ChatGPT] API error: %s", string(respBody))
		s.trackUsageFromContext(c, 0, 0, err)
		errorChunk := map[string]any{
			"error": map[string]any{
				"message": fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(respBody)),
				"type":    "api_error",
			},
		}
		errorJSON, _ := json.Marshal(errorChunk)
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		flusher.Flush()
		return
	}

	// Create SSE stream from the HTTP response using SDK's decoder
	// This properly handles all delta events from ChatGPT backend API
	sseStream := ssestream.NewStream[responses.ResponseStreamEventUnion](ssestream.NewDecoder(resp), nil)
	defer func() {
		if err := sseStream.Close(); err != nil {
			logrus.Errorf("[ChatGPT] Error closing stream: %v", err)
		}
	}()

	// Check if the original request was v1 or beta format
	// The v1 handler sets this context flag when routing through Responses API
	originalFormat := "beta"
	if fmt, exists := c.Get("original_request_format"); exists {
		if formatStr, ok := fmt.(string); ok {
			originalFormat = formatStr
		}
	}

	// Process the SSE stream using the proper handler based on original request format
	var streamErr error
	var usage protocol.UsageStat
	if originalFormat == "v1" {
		// Original request was v1 format, send response in v1 format
		usage, streamErr = streamhandler.HandleResponsesToAnthropicV1Stream(c, sseStream, responseModel)
	} else {
		// Original request was beta format, send response in beta format
		usage, streamErr = streamhandler.HandleResponsesToAnthropicBetaStream(c, sseStream, responseModel)
	}

	if streamErr != nil {
		s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, streamErr)
		logrus.Errorf("[ChatGPT] Stream handler error: %v", streamErr)
		if streamRec != nil {
			streamRec.RecordError(streamErr)
		}
		return
	}

	// Track usage from stream handler
	s.trackUsageFromContext(c, usage.InputTokens, usage.OutputTokens, nil)

	// Finish recording and assemble response
	if streamRec != nil {
		streamRec.Finish(responseModel, usage.InputTokens, usage.OutputTokens)
		streamRec.RecordResponse(provider, actualModel)
	}
}

// makeChatGPTBackendRequest creates and executes an HTTP request to ChatGPT backend API
func (s *Server) makeChatGPTBackendRequest(wrapper *client.OpenAIClient, provider *typ.Provider, params responses.ResponseNewParams) (*http.Response, context.CancelFunc, error) {
	// Convert OpenAI Responses API params to ChatGPT backend API format using protocol handler
	chatGPTReq := request.ConvertResponsesToChatGPTFormat(params, provider)

	bodyBytes, err := json.Marshal(chatGPTReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	logrus.Infof("[ChatGPT] Sending request to ChatGPT backend API: %s", string(bodyBytes))

	// Create HTTP request to ChatGPT backend API
	reqURL := provider.APIBase + "/codex/responses"
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for ChatGPT backend API
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	// Add ChatGPT-Account-ID header if available from OAuth metadata
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			req.Header.Set("ChatGPT-Account-ID", accountID)
		}
	}

	// Make the request
	resp, err := wrapper.HttpClient().Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, cancel, nil
}

// extractInstructions extracts system message content as instructions
func (s *Server) extractInstructions(raw map[string]interface{}) string {
	return request.ExtractInstructions(raw)
}

// convertInputToChatGPTFormat converts input items to ChatGPT backend API format
func (s *Server) convertInputToChatGPTFormat(raw map[string]interface{}) []interface{} {
	return request.ConvertRawInputToChatGPTFormat(raw)
}

// convertChatGPTResponseToOpenAI converts ChatGPT backend API response to OpenAI Responses API format
func (s *Server) convertChatGPTResponseToOpenAI(respBody []byte) (*responses.Response, error) {
	// Parse ChatGPT backend API response
	var chatGPTResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Status  string `json:"status"`
		Output  []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &chatGPTResp); err != nil {
		return nil, fmt.Errorf("failed to parse ChatGPT response: %w", err)
	}

	// Check for API error
	if chatGPTResp.Error != nil {
		return nil, fmt.Errorf("API error: %s (type: %s)", chatGPTResp.Error.Message, chatGPTResp.Error.Type)
	}

	// Convert response to JSON and unmarshal into OpenAI Response type
	respJSON, err := json.Marshal(chatGPTResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ChatGPT response: %w", err)
	}

	var openAIResp responses.Response
	if err := json.Unmarshal(respJSON, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to OpenAI response: %w", err)
	}

	return &openAIResp, nil
}

// convertChatGPTResponseToOpenAIChatCompletion converts a ChatGPT backend API response to OpenAI chat completion format
func (s *Server) convertChatGPTResponseToOpenAIChatCompletion(c *gin.Context, response responses.Response, responseModel string, inputTokens, outputTokens int64) {
	// Extract content from the response output
	var content string
	if len(response.Output) > 0 {
		for _, item := range response.Output {
			// Check if this is a message output
			if len(item.Content) > 0 {
				for _, c := range item.Content {
					if c.Type == "output_text" && c.Text != "" {
						content += c.Text
					}
				}
			}
		}
	}

	// Construct OpenAI chat completion response
	openAIResp := map[string]interface{}{
		"id":      response.ID,
		"object":  "chat.completion",
		"created": int64(response.CreatedAt),
		"model":   responseModel,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      inputTokens + outputTokens,
		},
	}

	c.JSON(http.StatusOK, openAIResp)
}
