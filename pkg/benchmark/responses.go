package benchmark

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
)

var (
	openaiResponseCounter    = 0
	anthropicResponseCounter = 0
	openaiCounterMutex       = &sync.Mutex{}
	anthropicCounterMutex    = &sync.Mutex{}
)

// Model represents a model in the models list
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// handleOpenAIModels handles the /v1/models endpoint
func (ms *MockServer) handleOpenAIModels(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	response := map[string]interface{}{
		"object": "list",
		"data":   ms.config.defaultModels,
	}
	c.JSON(http.StatusOK, response)
}

// handleAnthropicModels handles the /anthropic/v1/models endpoint
func (ms *MockServer) handleAnthropicModels(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	// Filter and return only Anthropic models in Anthropic's format
	var anthropicModels []map[string]interface{}
	for _, model := range ms.config.defaultModels {
		if model.OwnedBy == "anthropic" {
			anthropicModel := map[string]interface{}{
				"id":           model.ID,
				"display_name": model.ID,
				"type":         "model",
				"created_at":   model.Created,
			}
			anthropicModels = append(anthropicModels, anthropicModel)
		}
	}

	response := map[string]interface{}{
		"data": anthropicModels,
	}
	c.JSON(http.StatusOK, response)
}

// handleOpenAIChat handles the /v1/chat/completions endpoint
func (ms *MockServer) handleOpenAIChat(c *gin.Context) {
	ms.applyDelay(ms.config.chatDelayMs)

	response := ms.getChatResponse()
	c.JSON(http.StatusOK,  response)
}

// handleAnthropicMessages handles the /v1/messages endpoint
func (ms *MockServer) handleAnthropicMessages(c *gin.Context) {
	// Parse into MessageNewParams using SDK's JSON unmarshaling
	var req anthropic.MessageNewParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, "request do not follow anthropic api style" )
		return
	}

	ms.applyDelay(ms.config.msgDelayMs)

	response := ms.getMessageResponse()
	c.JSON(http.StatusOK, response)
}

// applyDelay applies delay to simulate real API latency
func (ms *MockServer) applyDelay(delayMs int) {
	if delayMs > 0 {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	} else if ms.config.randomDelayMinMs > 0 || ms.config.randomDelayMaxMs > 0 {
		min := ms.config.randomDelayMinMs
		max := ms.config.randomDelayMaxMs
		if min <= 0 {
			min = 0
		}
		if max <= min {
			max = min + 100
		}

		// Simple random delay (in production, use better randomness)
		delay := min + (max-min)/2
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

// getChatResponse returns the next chat response
func (ms *MockServer) getChatResponse() []byte {
	openaiCounterMutex.Lock()
	defer openaiCounterMutex.Unlock()

	if len(ms.config.defaultChatResponses) == 0 {
		return ms.getDefaultChatResponse()
	}

	index := openaiResponseCounter % len(ms.config.defaultChatResponses)
	response := ms.config.defaultChatResponses[index]

	if !ms.config.loopChatResponses && openaiResponseCounter >= len(ms.config.defaultChatResponses)-1 {
		// Keep returning the last response
	} else {
		openaiResponseCounter++
	}

	return response
}

// getMessageResponse returns the next message response
func (ms *MockServer) getMessageResponse() []byte {
	anthropicCounterMutex.Lock()
	defer anthropicCounterMutex.Unlock()

	if len(ms.config.defaultMsgResponses) == 0 {
		return ms.getDefaultMessageResponse()
	}

	index := anthropicResponseCounter % len(ms.config.defaultMsgResponses)
	response := ms.config.defaultMsgResponses[index]

	if !ms.config.loopMsgResponses && anthropicResponseCounter >= len(ms.config.defaultMsgResponses)-1 {
		// Keep returning the last response
	} else {
		anthropicResponseCounter++
	}

	return response
}

// getDefaultChatResponse returns a default chat response
func (ms *MockServer) getDefaultChatResponse() []byte {
	defaultResponse := map[string]interface{}{
		"id":      "chatcmpl-default",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-3.5-turbo",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "This is a default mock response from the benchmark server.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 15,
			"total_tokens":      25,
		},
	}

	data, _ := json.Marshal(defaultResponse)
	return data
}

// getDefaultMessageResponse returns a default message response
func (ms *MockServer) getDefaultMessageResponse() []byte {
	defaultResponse := map[string]interface{}{
		"id":   "msg-default",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "This is a default mock response from the benchmark server.",
			},
		},
		"model": "claude-3-sonnet-20240229",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 15,
		},
	}

	data, _ := json.Marshal(defaultResponse)
	return data
}

// getDefaultModels returns the default model list
func getDefaultModels() []Model {
	return []Model{
		{
			ID:      "gpt-3.5-turbo",
			Object:  "model",
			Created: 1677610602,
			OwnedBy: "openai",
		},
		{
			ID:      "gpt-4",
			Object:  "model",
			Created: 1687882411,
			OwnedBy: "openai",
		},
		{
			ID:      "gpt-4-turbo",
			Object:  "model",
			Created: 1712361441,
			OwnedBy: "openai",
		},
		{
			ID:      "claude-3-opus-20240229",
			Object:  "model",
			Created: 1706053735,
			OwnedBy: "anthropic",
		},
		{
			ID:      "claude-3-sonnet-20240229",
			Object:  "model",
			Created: 1706050349,
			OwnedBy: "anthropic",
		},
	}
}
