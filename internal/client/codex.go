package client

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

var codexInputIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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
	// ChatGPT backend API does NOT support: max_tokens, max_completion_tokens, temperature, top_p
	// It DOES support: max_output_tokens
	var isStreaming = true

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

func sanitizeCodexInputIDs(req map[string]interface{}) {
	input, ok := req["input"]
	if !ok {
		return
	}
	model, _ := req["model"].(string)
	req["input"] = sanitizeCodexValue(input, "input", model)
}

func sanitizeCodexValue(v interface{}, path, model string) interface{} {
	switch item := v.(type) {
	case []interface{}:
		for i := range item {
			item[i] = sanitizeCodexValue(item[i], fmt.Sprintf("%s[%d]", path, i), model)
		}
		return item
	case map[string]interface{}:
		for key, value := range item {
			item[key] = sanitizeCodexValue(value, path+"."+key, model)
		}
		if rawID, ok := item["id"].(string); ok {
			rawID = strings.TrimSpace(rawID)
			if rawID == "" || !codexInputIDPattern.MatchString(rawID) {
				delete(item, "id")
			} else {
				item["id"] = rawID
			}
		}
		return item
	default:
		return v
	}
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
