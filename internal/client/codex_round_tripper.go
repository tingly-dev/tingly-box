package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
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
		logrus.WithContext(req.Context()).Debugf("[Codex] Rewriting URL path: %s -> %s", originalPath, newPath)
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

	resp.Header.Set("Content-Type", "text/event-stream")
	logrus.WithContext(req.Context()).Debugf("[Codex] Must use stream: %s", resp.Status)

	return resp, nil
}

func (t *codexRoundTripper) filterField(body []byte) ([]byte, error) {
	// Filter the request body to remove unsupported parameters using sjson
	// This is more efficient than unmarshaling to map and marshaling back

	bodyStr := string(body)

	// codex require false here
	bodyStr, _ = sjson.SetRaw(bodyStr, "store", "false")
	bodyStr, _ = sjson.SetRaw(bodyStr, "stream", "true")

	// Remove unsupported parameters (ignore errors if key doesn't exist)
	bodyStr, _ = sjson.Delete(bodyStr, "max_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "max_completion_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "max_output_tokens")
	bodyStr, _ = sjson.Delete(bodyStr, "temperature")
	bodyStr, _ = sjson.Delete(bodyStr, "top_p")

	// Final gate: ChatGPT backend rejects items with empty or non-conforming id.
	// The SDK-level sanitizer only covers a subset of input item variants, so
	// scrub the marshaled JSON to catch every variant the SDK may emit.
	bodyStr = sanitizeCodexInputIDsJSON(bodyStr)

	return []byte(bodyStr), nil
}

// sanitizeCodexInputIDsJSON walks the input array and removes invalid ids.
// For types whose id is required, the entire item is dropped (the backend
// would reject the request anyway). For types whose id is optional, only
// the id field is removed.
func sanitizeCodexInputIDsJSON(bodyStr string) string {
	input := gjson.Get(bodyStr, "input")
	if !input.IsArray() {
		return bodyStr
	}

	items := input.Array()
	// Iterate in reverse so deletions don't shift earlier indices.
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		idVal := item.Get("id")
		if !idVal.Exists() {
			continue
		}
		idStr := strings.TrimSpace(idVal.String())
		if idStr != "" && isValidCodexID(idStr) {
			continue
		}

		itemType := item.Get("type").String()
		path := fmt.Sprintf("input.%d", i)
		if codexInputItemIDRequired(itemType) {
			logrus.Warnf("[Codex] Dropping input[%d] of type %q with invalid id %q", i, itemType, idStr)
			if updated, err := sjson.Delete(bodyStr, path); err == nil {
				bodyStr = updated
			}
		} else {
			logrus.Debugf("[Codex] Clearing invalid id on input[%d] type %q", i, itemType)
			if updated, err := sjson.Delete(bodyStr, path+".id"); err == nil {
				bodyStr = updated
			}
		}
	}
	return bodyStr
}

// codexInputItemIDRequired reports whether the given input item type requires
// the `id` field per the OpenAI Responses API schema. For these types, the
// SDK marshals an empty id as "" rather than omitting it, and the ChatGPT
// backend rejects it.
func codexInputItemIDRequired(itemType string) bool {
	switch itemType {
	case "reasoning",
		"code_interpreter_call",
		"computer_call",
		"file_search_call",
		"web_search_call",
		"image_generation_call",
		"local_shell_call",
		"local_shell_call_output",
		"mcp_list_tools",
		"mcp_approval_request",
		"mcp_call",
		"item_reference":
		return true
	}
	return false
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
