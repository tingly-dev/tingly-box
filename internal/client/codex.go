package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"
)

const reasoningMarker = "reasoning.encrypted_content"
const defaultInstructions = "You are a helpful AI assistant."

// codexRoundTripper wraps an http.RoundTripper to transform ChatGPT backend API
// responses to OpenAI Responses API format. The ChatGPT backend API returns a custom format
// that differs from the standard OpenAI Responses API spec.
//
// This RoundTripper transforms the response format to match what the OpenAI SDK expects.
type codexRoundTripper struct {
	http.RoundTripper
}

func (t *codexRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {

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
	// ChatGPT backend API does NOT support: max_tokens, max_completion_tokens, temperature, top_p, max_output_tokens

	var filtered []byte
	var isStreaming = false
	if req.Body != nil && req.Method == "POST" {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		filtered, err = t.filterField(body)
		if err != nil {
			return nil, fmt.Errorf("failed to filter field: %w", err)
		}

		// Trim capacity to length to avoid excessive memory usage
		filtered = append([]byte(nil), filtered...)
		// Set GetBody to allow retries and redirects
		req.Body = io.NopCloser(bytes.NewReader(filtered))
		req.ContentLength = int64(len(filtered))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(filtered)), nil
		}
	}

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
	} else {
		logrus.Warnf("[Codex] Do not support request without stream: %s", resp.Status)
	}

	return resp, nil
}

func (t *codexRoundTripper) filterField(body []byte) ([]byte, error) {
	// Filter the request body to remove unsupported parameters using sjson
	// This is more efficient than unmarshaling to map and marshaling back

	bodyStr := string(body)

	// Remove unsupported parameters (ignore errors if key doesn't exist)
	bodyStr, _ = sjson.Delete(bodyStr, "max_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "max_completion_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "max_output_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "temperature")
	bodyStr, _ = sjson.Delete(bodyStr, "top_p")

	return []byte(bodyStr), nil
}

func rewriteCodexPath(path string) string {
	if strings.HasPrefix(path, "/backend-api/") {
		return rewriteCodexAPIPath(path)
	}
	if strings.HasPrefix(path, "/v1/") && !strings.Contains(path, "/codex/") {
		return strings.Replace(path, "/v1/", "/codex/", 1)
	}
	return path
}

func rewriteCodexAPIPath(path string) string {
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
