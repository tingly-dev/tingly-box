package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// chatGPTBackendRoundTripper wraps an http.RoundTripper to transform ChatGPT backend API
// responses to OpenAI Responses API format. The ChatGPT backend API returns a custom format
// that differs from the standard OpenAI Responses API spec.
//
// This RoundTripper transforms the response format to match what the OpenAI SDK expects.
type chatGPTBackendRoundTripper struct {
	http.RoundTripper
}

// chatGPTBackendStreamingReader wraps an io.ReadCloser to transform ChatGPT backend API
// SSE events to OpenAI Responses API format on-the-fly.
type chatGPTBackendStreamingReader struct {
	reader io.ReadCloser
	buffer []byte
	err    error
}

func (t *chatGPTBackendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {

	// codexHook applies ChatGPT/Codex OAuth specific request modifications:
	// - Rewrites URL paths from /v1/... to /codex/... for ChatGPT backend API
	// - Handles special cases for responses endpoint
	// - Adds required ChatGPT backend API headers
	// - Transforms X-ChatGPT-Account-ID to ChatGPT-Account-ID header
	originalPath := req.URL.Path
	newPath := rewriteCodexPath(originalPath)

	if newPath != originalPath {
		logrus.Debugf("[Codex] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	req.Header.Set("OpenAI-Beta", "responses=experimental")
	//req.Header.Set("originator", "tingly-box")

	if accountID := req.Header.Get("X-ChatGPT-Account-ID"); accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
		req.Header.Del("X-ChatGPT-Account-ID")
	}

	// Filter out unsupported parameters for ChatGPT backend API
	// ChatGPT backend API does NOT support: max_tokens, max_completion_tokens, temperature, top_p
	// It DOES support: max_output_tokens
	var filtered []byte
	var isStreaming = false
	if req.Body != nil && req.Method == "POST" {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		// Filter the request body to remove unsupported parameters
		filtered, isStreaming = filterChatGPTBackendRequestJSON(body)
		// Trim capacity to length to avoid excessive memory usage
		filtered = append([]byte(nil), filtered...)
		// Set GetBody to allow retries and redirects
		req.Body = io.NopCloser(bytes.NewReader(filtered))
		req.ContentLength = int64(len(filtered))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(filtered)), nil
		}
	}

	//if isStreaming {
	//	req.Header.Set("Content-Type", "text/event-stream")
	//}

	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(errorBody))
	}

	if isStreaming {
		resp.Header.Set("Content-Type", "text/event-stream")
		//resp.Body = &chatGPTBackendStreamingReader{reader: resp.Body}
	} else {
		// For non-streaming, transform the entire response body
		body, err := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			logrus.Warnf("[Codex] Error closing response body: %v", closeErr)
		}
		if err != nil {
			return nil, err
		}

		// Transform ChatGPT backend format to OpenAI Responses format
		transformed := transformChatGPTBackendResponseJSON(body)
		resp.Body = io.NopCloser(bytes.NewReader(transformed))
		resp.ContentLength = int64(len(transformed))
	}

	return resp, nil
}

func (r *chatGPTBackendStreamingReader) Read(p []byte) (n int, err error) {
	if len(r.buffer) > 0 {
		n = copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		return n, nil
	}

	if r.err != nil {
		return 0, r.err
	}

	buf := make([]byte, 4096)
	var lineBuffer bytes.Buffer

	for {
		nn, readErr := r.reader.Read(buf)
		if nn > 0 {
			lineBuffer.Write(buf[:nn])
		}
		if readErr != nil {
			if readErr == io.EOF {
				if lineBuffer.Len() > 0 {
					r.buffer = r.processChatGPTBuffer(lineBuffer.Bytes())
					n = copy(p, r.buffer)
					r.buffer = r.buffer[n:]
					if len(r.buffer) == 0 {
						r.err = io.EOF
					}
					return n, nil
				}
				return 0, io.EOF
			}
			r.err = readErr
			return 0, readErr
		}

		processed := r.processChatGPTBuffer(lineBuffer.Bytes())
		if len(processed) > 0 {
			n = copy(p, processed)
			r.buffer = processed[n:]
			return n, nil
		}

		if lineBuffer.Len() > 40960 {
			n = copy(p, lineBuffer.Bytes())
			lineBuffer.Next(n)
			return n, nil
		}
	}
}

func (r *chatGPTBackendStreamingReader) processChatGPTBuffer(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	var result bytes.Buffer

	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("data:")) {
			jsonData := bytes.TrimSpace(line[5:])
			if len(jsonData) == 0 {
				result.Write(line)
			} else if bytes.Equal(jsonData, []byte("[DONE]")) {
				result.Write(line)
			} else {
				transformed := transformChatGPTBackendChunkJSON(jsonData)
				if len(transformed) > 0 {
					result.Write([]byte("data: "))
					result.Write(transformed)
				} else {
					result.Write(line)
				}
			}
		} else {
			result.Write(line)
		}

		if i < len(lines)-1 || data[len(data)-1] == '\n' {
			result.WriteByte('\n')
		}
	}

	return result.Bytes()
}

func (r *chatGPTBackendStreamingReader) Close() error {
	return r.reader.Close()
}

// transformChatGPTBackendChunkJSON transforms a single ChatGPT backend SSE chunk to OpenAI format.
// Returns empty bytes if transformation fails (original will be used as fallback).
func transformChatGPTBackendChunkJSON(jsonData []byte) []byte {
	var chatGPTChunk struct {
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
			} `json:"output"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
				TotalTokens  int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}

	if err := json.Unmarshal(jsonData, &chatGPTChunk); err != nil {
		return nil
	}

	openAIChunk := map[string]interface{}{
		"type": chatGPTChunk.Type,
	}

	if chatGPTChunk.Response != nil {
		response := map[string]interface{}{
			"id":         chatGPTChunk.Response.ID,
			"created_at": chatGPTChunk.Response.CreatedAt,
			"status":     "in_progress",
		}

		if len(chatGPTChunk.Response.Output) > 0 {
			output := make([]map[string]interface{}, 0)
			for _, item := range chatGPTChunk.Response.Output {
				if item.Type == "message" {
					content := make([]map[string]interface{}, 0)
					for _, c := range item.Content {
						if c.Type == "output_text" {
							content = append(content, map[string]interface{}{
								"type": "output_text",
								"text": c.Text,
							})
						}
					}
					output = append(output, map[string]interface{}{
						"type":    "message",
						"role":    "assistant",
						"content": content,
					})
				}
			}
			response["output"] = output
		}

		if chatGPTChunk.Response.Usage != nil {
			response["usage"] = map[string]interface{}{
				"input_tokens":  chatGPTChunk.Response.Usage.InputTokens,
				"output_tokens": chatGPTChunk.Response.Usage.OutputTokens,
				"total_tokens":  chatGPTChunk.Response.Usage.TotalTokens,
			}
		}

		openAIChunk["response"] = response
	}

	result, _ := json.Marshal(openAIChunk)
	return result
}

// transformChatGPTBackendResponseJSON transforms a non-streaming ChatGPT backend response to OpenAI format.
func transformChatGPTBackendResponseJSON(data []byte) []byte {
	var chatGPTResp struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Created int64  `json:"created"`
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
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &chatGPTResp); err != nil {
		return data // Return original if parsing fails
	}

	openAIResp := map[string]interface{}{
		"id":         chatGPTResp.ID,
		"object":     "response",
		"created_at": chatGPTResp.Created,
		"status":     chatGPTResp.Status,
	}

	if len(chatGPTResp.Output) > 0 {
		output := make([]map[string]interface{}, 0)
		for _, item := range chatGPTResp.Output {
			if item.Type == "message" {
				content := make([]map[string]interface{}, 0)
				for _, c := range item.Content {
					if c.Type == "output_text" {
						content = append(content, map[string]interface{}{
							"type": "output_text",
							"text": c.Text,
						})
					}
				}
				output = append(output, map[string]interface{}{
					"type":    "message",
					"role":    "assistant",
					"content": content,
				})
			}
		}
		openAIResp["output"] = output
	}

	openAIResp["usage"] = map[string]interface{}{
		"input_tokens":  chatGPTResp.Usage.InputTokens,
		"output_tokens": chatGPTResp.Usage.OutputTokens,
		"total_tokens":  chatGPTResp.Usage.TotalTokens,
	}

	result, _ := json.Marshal(openAIResp)
	return result
}

// filterChatGPTBackendRequestJSON filters out unsupported parameters for ChatGPT backend API.
// ChatGPT backend API does NOT support: max_tokens, max_completion_tokens, temperature, top_p
// It DOES support: max_output_tokens
func filterChatGPTBackendRequestJSON(data []byte) ([]byte, bool) {
	var req map[string]interface{}
	if err := json.Unmarshal(data, &req); err != nil {
		return data, false // Return original if parsing fails
	}

	isStreaming := false
	if stream, ok := req["stream"]; ok {
		isStreaming, ok = stream.(bool)
	}

	// required
	req["store"] = false
	req["include"] = []string{}

	if ins, ok := req["instructions"]; ok {
		if insStr, ok := ins.(string); ok {
			req["instructions"] = "You are a helpful AI assistant."
			if input, ok := req["input"].([]any); ok {
				tmp := []any{
					map[string]any{
						"content": []map[string]any{
							{
								"text": insStr,
								"type": "input_text",
							},
						},
						"role": "user", "type": "message"},
				}
				if len(input) > 0 {
					tmp = append(tmp, input...)
				}
				req["input"] = tmp
			}
		}
	}
	//delete(req, "tools")
	//delete(req, "input")

	// Filter out unsupported parameters
	// These parameters are NOT supported by ChatGPT backend API
	delete(req, "max_tokens")
	delete(req, "max_completion_tokens")
	delete(req, "max_output_tokens")
	delete(req, "temperature")
	delete(req, "top_p")

	// max_output_tokens IS supported, so we keep it
	// Other supported parameters: instructions, input, tools, tool_choice, stream, store, include, etc.

	result, err := json.Marshal(req)
	if err != nil {
		return data, isStreaming // Return original if marshaling fails
	}
	return result, isStreaming
}

func rewriteCodexPath(path string) string {
	if strings.HasPrefix(path, "/backend-api/") {
		return rewriteBackendAPIPath(path)
	}
	if strings.HasPrefix(path, "/v1/") && !strings.Contains(path, "/codex/") {
		return strings.Replace(path, "/v1/", "/codex/", 1)
	}
	return path
}

func rewriteBackendAPIPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/backend-api/chat/completions"):
		return "/backend-api/codex/responses"
	case path == "/backend-api/responses":
		return "/backend-api/codex/responses"
	case strings.HasPrefix(path, "/backend-api/v1/"):
		return strings.Replace(path, "/backend-api/v1/", "/backend-api/codex/", 1)
	default:
		return path
	}
}
