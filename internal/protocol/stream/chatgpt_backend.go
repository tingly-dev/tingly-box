package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
)

// ChatGPTBackendStreamResult represents the accumulated result from a ChatGPT backend stream.
type ChatGPTBackendStreamResult struct {
	ResponseID   string
	Created      int64
	InputTokens  int
	OutputTokens int
	Content      string
}

// AccumulateChatGPTBackendStream reads SSE stream from ChatGPT backend API and accumulates into a result.
func AccumulateChatGPTBackendStream(reader io.Reader) (*ChatGPTBackendStreamResult, error) {
	var fullOutput strings.Builder
	var inputTokens, outputTokens int
	var responseID string
	var created int64

	logrus.Infof("[ChatGPT] Reading streaming response from ChatGPT backend API")
	scanner := bufio.NewScanner(reader)
	// Increase buffer size to handle large SSE chunks (default 64KB is too small)
	scanner.Buffer(nil, bufio.MaxScanTokenSize<<9) // 32MB buffer
	chunkCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			logrus.Infof("[ChatGPT] Received [DONE] signal")
			break
		}

		if chunkCount < 3 {
			logrus.Infof("[ChatGPT] SSE chunk #%d: %s", chunkCount+1, jsonData)
		}

		var chunk struct {
			Type     string `json:"type"`
			Response *struct {
				ID        string `json:"id"`
				CreatedAt int64  `json:"created_at"`
				Output    []struct {
					ID      string `json:"id"`
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
					Summary []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"summary"`
				} `json:"output"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			logrus.Warnf("[ChatGPT] Failed to parse SSE chunk: %s, data: %s", err, jsonData)
			continue
		}

		chunkCount++

		if chunk.Response != nil {
			if chunk.Response.ID != "" {
				responseID = chunk.Response.ID
			}
			if chunk.Response.CreatedAt > 0 {
				created = chunk.Response.CreatedAt
			}

			for _, item := range chunk.Response.Output {
				if item.Type == "message" {
					for _, content := range item.Content {
						if content.Type == "output_text" {
							fullOutput.WriteString(content.Text)
							logrus.Debugf("[ChatGPT] Accumulated content length: %d, text: %s", fullOutput.Len(), content.Text)
						} else if content.Type == "refusal" {
							logrus.Warnf("[ChatGPT] Refusal content detected: %s", content.Text)
							fullOutput.WriteString(content.Text)
						}
					}
				} else {
					logrus.Debugf("[ChatGPT] Skipping output item type: %s, id: %s", item.Type, item.ID)
				}
			}

			if chunk.Response.Usage != nil {
				if chunk.Response.Usage.InputTokens > 0 {
					inputTokens = chunk.Response.Usage.InputTokens
				}
				if chunk.Response.Usage.OutputTokens > 0 {
					outputTokens = chunk.Response.Usage.OutputTokens
				}
			}
		}
	}

	logrus.Infof("[ChatGPT] Finished reading SSE stream: %d chunks, output length: %d, tokens: %d in, %d out", chunkCount, fullOutput.Len(), inputTokens, outputTokens)

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read streaming response: %w", err)
	}

	if responseID == "" {
		responseID = "chatgpt-" + fmt.Sprintf("%d", time.Now().Unix())
	}
	if created == 0 {
		created = time.Now().Unix()
	}

	return &ChatGPTBackendStreamResult{
		ResponseID:   responseID,
		Created:      created,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Content:      fullOutput.String(),
	}, nil
}

// ConvertStreamResultToResponse converts a ChatGPTBackendStreamResult to OpenAI Response format.
func ConvertStreamResultToResponse(result *ChatGPTBackendStreamResult, model string) (*responses.Response, error) {
	resultMap := map[string]interface{}{
		"id":         result.ResponseID,
		"object":     "response",
		"created_at": float64(result.Created),
		"model":      model,
		"status":     "completed",
		"usage": map[string]interface{}{
			"input_tokens":  result.InputTokens,
			"output_tokens": result.OutputTokens,
			"total_tokens":  result.InputTokens + result.OutputTokens,
		},
	}

	if result.Content != "" {
		resultMap["output"] = []map[string]interface{}{
			{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]string{
					{
						"type": "output_text",
						"text": result.Content,
					},
				},
			},
		}
	}

	resultJSON, _ := json.Marshal(resultMap)
	var resp responses.Response
	if err := json.Unmarshal(resultJSON, &resp); err != nil {
		return nil, fmt.Errorf("failed to construct response: %w", err)
	}

	return &resp, nil
}
