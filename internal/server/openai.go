package server

import (
	"encoding/json"
	"net/http"
	"time"
	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// OpenAIChatCompletions handles OpenAI v1 chat completion requests
func (s *Server) OpenAIChatCompletions(c *gin.Context) {
	// Use the existing ChatCompletions logic for OpenAI compatibility
	s.ChatCompletions(c)
}

// ChatCompletions handles OpenAI-compatible chat completion requests
func (s *Server) ChatCompletions(c *gin.Context) {
	// Read the raw request body to check for stream parameter
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Error("Failed to read request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		logrus.Error("Invalid JSON in request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid JSON: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if streaming is requested
	isStreaming := false
	if stream, ok := rawReq["stream"].(bool); ok {
		isStreaming = stream
	}
	logrus.Infof("Stream requested: %v", isStreaming)

	// Parse request body into RequestWrapper
	var req RequestWrapper
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		logrus.Error("Invalid request body")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	for i := 0; i < len(req.Messages); i++ {
		logrus.Infof("messages: %s %s", *req.Messages[i].GetRole(), req.Messages[i].GetContent())
	}

	// Validate required fields
	if req.Model == "" {
		logrus.Error("No model id")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		logrus.Error("No messages")
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "At least one message is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	provider, modelDef, err := s.DetermineProviderAndModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Update request with actual model name if we have model definition
	if modelDef != nil {
		req.Model = modelDef.Model
	}

	// Handle response model modification at JSON level
	globalConfig := s.config.GetGlobalConfig()
	responseModel := ""
	if globalConfig != nil {
		responseModel = globalConfig.GetResponseModel()
	}
	if responseModel == "" {
		responseModel = req.Model
	}

	// Handle streaming or non-streaming request
	if isStreaming {
		s.handleStreamingRequest(c, provider, &req, responseModel)
	} else {
		s.handleNonStreamingRequest(c, provider, &req, responseModel)
	}
}

// handleStreamingRequest handles streaming chat completion requests
func (s *Server) handleStreamingRequest(c *gin.Context, provider *config.Provider, req *RequestWrapper, responseModel string) {
	// Create streaming request
	stream, err := s.forwardOpenAIStreamRequest(provider, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to create streaming request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Handle the streaming response
	s.handleOpenAIStreamResponse(c, stream, responseModel)
}

// handleNonStreamingRequest handles non-streaming chat completion requests
func (s *Server) handleNonStreamingRequest(c *gin.Context, provider *config.Provider, req *RequestWrapper, responseModel string) {
	// Forward request to provider
	response, err := s.forwardOpenAIRequest(provider, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

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

	// Return modified response
	c.JSON(http.StatusOK, responseMap)
}

// ListModels handles the /v1/models endpoint (OpenAI compatible)
func (s *Server) ListModels(c *gin.Context) {
	if s.providerManager == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model manager not available",
				Type:    "internal_error",
			},
		})
		return
	}

	models := s.providerManager.GetAllModels()

	// Convert to OpenAI-compatible format
	var openaiModels []map[string]interface{}
	for _, model := range models {
		openaiModel := map[string]interface{}{
			"id":       model.Name,
			"object":   "model",
			"created":  time.Now().Unix(), // In a real implementation, use actual creation date
			"owned_by": "tingly-box",
		}

		// Add permission information
		var permissions []string
		if model.Category == "chat" {
			permissions = append(permissions, "chat.completions")
		}

		openaiModel["permission"] = permissions

		// Add aliases as metadata
		if len(model.Aliases) > 0 {
			openaiModel["metadata"] = map[string]interface{}{
				"provider":     model.Provider,
				"api_base":     model.APIBase,
				"actual_model": model.Model,
				"aliases":      model.Aliases,
				"description":  model.Description,
				"category":     model.Category,
			}
		} else {
			openaiModel["metadata"] = map[string]interface{}{
				"provider":     model.Provider,
				"api_base":     model.APIBase,
				"actual_model": model.Model,
				"description":  model.Description,
				"category":     model.Category,
			}
		}

		openaiModels = append(openaiModels, openaiModel)
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   openaiModels,
	}

	c.JSON(http.StatusOK, response)
}
