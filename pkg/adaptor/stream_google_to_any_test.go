package adaptor

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

// TestHandleGoogleToOpenAIStreamResponse tests Google to OpenAI streaming response conversion
func TestHandleGoogleToOpenAIStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("text streaming response", func(t *testing.T) {
		// Create mock Google stream
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			// Initial response with text
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("Hello"),
							},
						},
					},
				},
			}, nil)
			// Second chunk
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText(" world"),
							},
						},
					},
				},
			}, nil)
			// Final chunk with finish reason
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("!"),
							},
						},
						FinishReason: genai.FinishReasonStop,
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

		// Verify SSE format
		body := w.Body.String()
		assert.Contains(t, body, "data: ")
		assert.Contains(t, body, "Hello")
		assert.Contains(t, body, "world")
		assert.Contains(t, body, "finish_reason")
		assert.Contains(t, body, "[DONE]")
	})

	t.Run("streaming with tool calls", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			// Response with function call
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "call_123",
										Name: "get_weather",
										Args: map[string]interface{}{"city": "NYC"},
									},
								},
							},
						},
					},
				},
			}, nil)
			// Final chunk
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: []*genai.Part{},
						},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, "tool_calls")
		assert.Contains(t, body, "get_weather")
		assert.Contains(t, body, "call_123")
		assert.Contains(t, body, "NYC")
	})

	t.Run("streaming error handling", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: []*genai.Part{},
						},
					},
				},
			}, nil)
			// Simulate error
			yield(nil, assert.AnError)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err) // Error is handled internally
	})

	t.Run("max tokens finish reason", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: []*genai.Part{},
						},
						FinishReason: genai.FinishReasonMaxTokens,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, `"finish_reason":"length"`)
	})

	t.Run("multiple tool calls in single response", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "call_1",
										Name: "get_weather",
										Args: map[string]interface{}{"city": "NYC"},
									},
								},
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "call_2",
										Name: "get_weather",
										Args: map[string]interface{}{"city": "Tokyo"},
									},
								},
							},
						},
					},
				},
			}, nil)
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, "call_1")
		assert.Contains(t, body, "call_2")
		// Should have "tool_calls" as finish reason when there are tool calls
		assert.Contains(t, body, `"finish_reason":"tool_calls"`)
	})
}

// TestHandleGoogleToAnthropicStreamResponse tests Google to Anthropic streaming response conversion
func TestHandleGoogleToAnthropicStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("text streaming response", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			// Initial text
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("Hello"),
							},
						},
					},
				},
			}, nil)
			// More text
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText(" world"),
							},
						},
					},
				},
			}, nil)
			// Final with finish reason
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: []*genai.Part{},
						},
						FinishReason: genai.FinishReasonStop,
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

		body := w.Body.String()
		// Should have Anthropic SSE format
		assert.Contains(t, body, "event: message_start")
		assert.Contains(t, body, "event: content_block_start")
		assert.Contains(t, body, "event: content_block_delta")
		assert.Contains(t, body, "event: content_block_stop")
		assert.Contains(t, body, "event: message_delta")
		assert.Contains(t, body, "event: message_stop")
		assert.Contains(t, body, "Hello")
		assert.Contains(t, body, "world")
	})

	t.Run("streaming with tool use", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			// Tool use
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "toolu_123",
										Name: "search",
										Args: map[string]interface{}{"query": "test"},
									},
								},
							},
						},
					},
				},
			}, nil)
			// Final
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, `"type":"tool_use"`)
		assert.Contains(t, body, "toolu_123")
		assert.Contains(t, body, "search")
		assert.Contains(t, body, "test")
	})

	t.Run("streaming error event", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(nil, assert.AnError)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, "event: error")
		assert.Contains(t, body, "stream_error")
	})

	t.Run("max tokens stop reason", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonMaxTokens,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, `"stop_reason":"max_tokens"`)
	})

	t.Run("text and tool use mixed response", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			// First text
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("I'll search"),
							},
						},
					},
				},
			}, nil)
			// Tool use
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "toolu_456",
										Name: "search",
										Args: map[string]interface{}{"q": "test"},
									},
								},
							},
						},
					},
				},
			}, nil)
			// Final
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, "I'll search")
		assert.Contains(t, body, "tool_use")
		assert.Contains(t, body, "search")
	})
}

// TestHandleGoogleToAnthropicBetaStreamResponse tests Google to Anthropic beta streaming response conversion
func TestHandleGoogleToAnthropicBetaStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("basic streaming with text", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("Test"),
							},
						},
					},
				},
			}, nil)
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     5,
					CandidatesTokenCount: 3,
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicBetaStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		// Beta format uses different event types
		assert.Contains(t, body, "message_start")
		assert.Contains(t, body, "content_block_start")
		assert.Contains(t, body, "content_block_delta")
		assert.Contains(t, body, "message_stop")
		assert.Contains(t, body, "Test")
	})

	t.Run("finish reason mapping for beta", func(t *testing.T) {
		tests := []struct {
			name         string
			finishReason genai.FinishReason
			expected     string
		}{
			{"stop", genai.FinishReasonStop, "end_turn"},
			{"max_tokens", genai.FinishReasonMaxTokens, "max_tokens"},
			{"safety", genai.FinishReasonSafety, "refusal"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
					yield(&genai.GenerateContentResponse{
						Candidates: []*genai.Candidate{
							{
								Content:      &genai.Content{},
								FinishReason: tt.finishReason,
							},
						},
					}, nil)
				}

				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("GET", "/", nil)

				err := HandleGoogleToAnthropicBetaStreamResponse(c, stream, "gemini-pro")

				assert.NoError(t, err)
				body := w.Body.String()
				assert.Contains(t, body, tt.expected)
			})
		}
	})

	t.Run("multiple tool uses", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "toolu_1",
										Name: "func1",
										Args: map[string]interface{}{"arg": "1"},
									},
								},
								{
									FunctionCall: &genai.FunctionCall{
										ID:   "toolu_2",
										Name: "func2",
										Args: map[string]interface{}{"arg": "2"},
									},
								},
							},
						},
					},
				},
			}, nil)
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		err := HandleGoogleToAnthropicBetaStreamResponse(c, stream, "gemini-pro")

		assert.NoError(t, err)
		body := w.Body.String()
		assert.Contains(t, body, "func1")
		assert.Contains(t, body, "func2")
		assert.Contains(t, body, "toolu_1")
		assert.Contains(t, body, "toolu_2")
	})
}

// TestStreamChunkFormatValidation validates SSE chunk format
func TestStreamChunkFormatValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("OpenAI SSE format validation", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("Hi"),
							},
						},
					},
				},
			}, nil)
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

		body := w.Body.String()
		// Check SSE format
		assert.Contains(t, body, "data: {")
		assert.Contains(t, body, "\n\n")
		// Check required fields
		assert.Contains(t, body, `"id"`)
		assert.Contains(t, body, `"object":"chat.completion.chunk"`)
		assert.Contains(t, body, `"model":"gemini-pro"`)
		assert.Contains(t, body, `"created"`)
	})

	t.Run("Anthropic SSE format validation", func(t *testing.T) {
		stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText("Hi"),
							},
						},
					},
				},
			}, nil)
			yield(&genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}, nil)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		HandleGoogleToAnthropicStreamResponse(c, stream, "gemini-pro")

		body := w.Body.String()
		// Check Anthropic SSE format
		assert.Contains(t, body, "event: ")
		assert.Contains(t, body, "data: ")
		assert.Contains(t, body, "\n\n")
		// Check message structure
		assert.Contains(t, body, `"type":"message"`)
		assert.Contains(t, body, `"role":"assistant"`)
		assert.Contains(t, body, `"stop_reason"`)
	})
}

// TestStreamIterHelper is a helper to create iter.Seq2 from a slice
func TestStreamIterHelper(t *testing.T) {
	// This test creates a proper iter.Seq2 for testing
	responses := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("Test"),
						},
					},
				},
			},
		},
		{
			Candidates: []*genai.Candidate{
				{
					Content:      &genai.Content{},
					FinishReason: genai.FinishReasonStop,
				},
			},
		},
	}

	stream := func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, resp := range responses {
			if !yield(resp, nil) {
				return
			}
		}
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	err := HandleGoogleToOpenAIStreamResponse(c, stream, "gemini-pro")

	assert.NoError(t, err)
	assert.Contains(t, w.Body.String(), "Test")
}
