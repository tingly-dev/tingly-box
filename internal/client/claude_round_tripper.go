package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// claudeToolPrefix is empty to match real Claude Code behavior (no tool name prefix).
const claudeToolPrefix = ""

// oauthToolRenameMap maps lowercase tool names to Claude Code TitleCase equivalents.
// Anthropic uses tool name fingerprinting to detect third-party clients on OAuth traffic.
// Renaming to official names avoids extra-usage billing.
var oauthToolRenameMap = map[string]string{
	"bash":         "Bash",
	"read":         "Read",
	"write":        "Write",
	"edit":         "Edit",
	"glob":         "Glob",
	"grep":         "Grep",
	"task":         "Task",
	"webfetch":     "WebFetch",
	"todowrite":    "TodoWrite",
	"question":     "Question",
	"skill":        "Skill",
	"ls":           "LS",
	"todoread":     "TodoRead",
	"notebookedit": "NotebookEdit",
}

const (
	// Claude Code client identification
	claudeCLIUserAgent      = "claude-cli/2.1.86 (external, cli)"
	claudeXApp              = "cli"
	stainlessHelperMethod   = "stream"
	stainlessRetryCount     = "0"
	stainlessRuntimeVersion = "v24.3.0"
	stainlessPackageVersion = "0.74.0"
	stainlessRuntime        = "node"
	stainlessLang           = "js"
	stainlessTimeout        = "600"

	// Anthropic API headers
	anthropicBeta                         = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28"
	anthropicOAuthBeta                    = "oauth-2025-04-20"
	anthropicDangerousDirectBrowserAccess = "true"
	anthropicVersion                      = "2023-06-01"

	// Model-specific beta flags
	anthropicContext1m = "context-1m-2025-08-07"

	// Content negotiation
	acceptHeader = "application/json"

	// Buffer sizes
	maxStreamingLineSize = 52_428_800 // 50MB max line size
)

// stainlessOS returns the OS name for the x-stainless-os header
func stainlessOS() string {
	return runtime.GOOS // e.g., "darwin", "linux", "windows"
}

// stainlessArch returns the architecture for the x-stainless-arch header
func stainlessArch() string {
	return runtime.GOARCH // e.g., "amd64", "arm64"
}

// claudeModelPrefixes that support context-1m beta flag.
var context1mModelPrefixes = []string{
	"claude-sonnet-4-6",
	"claude-opus-4-6",
}

// supportsContext1M checks if the model supports the context-1m-2025-08-07 beta flag.
func supportsContext1M(model string) bool {
	m := strings.ToLower(model)
	for _, prefix := range context1mModelPrefixes {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// IsClaudeOAuthToken checks if the given API key is a Claude OAuth token
// by checking for the "sk-ant-oat" prefix.
func IsClaudeOAuthToken(apiKey string) bool {
	return apiKey != "" && strings.Contains(apiKey, "sk-ant-oat")
}

// extractSessionIDFromBody extracts the session_id from the request body's
// metadata.user_id field. The user_id field has two variants:
//   - JSON: {"device_id":"...","account_uuid":"...","session_id":"..."}
//   - Legacy string: user_{64hex}_account_{uuid}_session_{uuid}
func extractSessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	raw := gjson.GetBytes(body, "metadata.user_id").String()
	if raw == "" {
		return ""
	}
	m := ops.ParseMetadataUserID(raw)
	if m == nil {
		return ""
	}
	return m.SessionID
}

// extractModelFromBody parses the "model" field from JSON body without full unmarshal.
func extractModelFromBody(body []byte) string {
	return gjson.GetBytes(body, "model").String()
}

// remapOAuthToolNames renames lowercase tool names to Claude Code TitleCase equivalents
// in the request body. Returns the modified body and a reverse map for restoring names
// in the response (keyed on TitleCase → original name, only for names actually renamed).
func remapOAuthToolNames(body []byte) ([]byte, map[string]string) {
	reverseMap := make(map[string]string)
	record := func(original, renamed string) {
		if _, exists := reverseMap[renamed]; !exists {
			reverseMap[renamed] = original
		}
	}

	// Rewrite tools array
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() {
		var sb strings.Builder
		sb.WriteByte('[')
		count := 0
		tools.ForEach(func(_, tool gjson.Result) bool {
			// Leave built-in tools (have a "type" field) unchanged
			if tool.Get("type").Exists() && tool.Get("type").String() != "" {
				if count > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(tool.Raw)
				count++
				return true
			}
			name := tool.Get("name").String()
			toolJSON := tool.Raw
			if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
				if updated, err := sjson.Set(toolJSON, "name", newName); err == nil {
					toolJSON = updated
					record(name, newName)
				}
			}
			if count > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(toolJSON)
			count++
			return true
		})
		sb.WriteByte(']')
		body, _ = sjson.SetRawBytes(body, "tools", []byte(sb.String()))
	}

	// Rewrite tool_choice if it names a specific tool
	if gjson.GetBytes(body, "tool_choice.type").String() == "tool" {
		name := gjson.GetBytes(body, "tool_choice.name").String()
		if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
			body, _ = sjson.SetBytes(body, "tool_choice.name", newName)
			record(name, newName)
		}
	}

	// Rewrite tool references in messages
	messages := gjson.GetBytes(body, "messages")
	if messages.Exists() && messages.IsArray() {
		messages.ForEach(func(msgIdx, msg gjson.Result) bool {
			content := msg.Get("content")
			if !content.Exists() || !content.IsArray() {
				return true
			}
			content.ForEach(func(cIdx, part gjson.Result) bool {
				switch part.Get("type").String() {
				case "tool_use":
					name := part.Get("name").String()
					if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
						path := fmt.Sprintf("messages.%d.content.%d.name", msgIdx.Int(), cIdx.Int())
						body, _ = sjson.SetBytes(body, path, newName)
						record(name, newName)
					}
				case "tool_reference":
					name := part.Get("tool_name").String()
					if newName, ok := oauthToolRenameMap[name]; ok && newName != name {
						path := fmt.Sprintf("messages.%d.content.%d.tool_name", msgIdx.Int(), cIdx.Int())
						body, _ = sjson.SetBytes(body, path, newName)
						record(name, newName)
					}
				}
				return true
			})
			return true
		})
	}

	return body, reverseMap
}

// reverseRemapOAuthToolNames restores tool names in non-stream responses using
// the per-request reverseMap produced by remapOAuthToolNames.
func reverseRemapOAuthToolNames(body []byte, reverseMap map[string]string) []byte {
	if len(reverseMap) == 0 {
		return body
	}
	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return body
	}
	content.ForEach(func(idx, part gjson.Result) bool {
		switch part.Get("type").String() {
		case "tool_use":
			name := part.Get("name").String()
			if orig, ok := reverseMap[name]; ok {
				path := fmt.Sprintf("content.%d.name", idx.Int())
				body, _ = sjson.SetBytes(body, path, orig)
			}
		case "tool_reference":
			name := part.Get("tool_name").String()
			if orig, ok := reverseMap[name]; ok {
				path := fmt.Sprintf("content.%d.tool_name", idx.Int())
				body, _ = sjson.SetBytes(body, path, orig)
			}
		}
		return true
	})
	return body
}

// claudeRoundTripper wraps an http.RoundTripper to handle Claude Code OAuth
// - Applies tool prefix to request body for OAuth tokens
// - Strips tool prefix from response (streaming and non-streaming)
// - Sets Claude Code specific headers
// - Manages conditional Authorization vs x-api-key header
type claudeRoundTripper struct {
	http.RoundTripper
}

func (t *claudeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Reject /models endpoint for Claude Code OAuth (by design)
	if req.URL != nil && strings.HasSuffix(req.URL.Path, "/models") && req.Method == http.MethodGet {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     http.StatusText(http.StatusNotFound),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"type":"not_found_error","message":"models endpoint is not supported for Claude Code"}}`))),
		}, nil
	}

	// claudeHook applies Claude Code OAuth specific request modifications:
	// - Detects OAuth token (sk-ant-oat prefix)
	// - Applies tool prefix to request body for OAuth tokens
	// - Sets Claude Code specific headers with conditional auth
	// - Adds beta query parameter

	// Extract and read request body for potential modification
	var originalBody []byte
	var modifiedBody []byte
	var isOAuthToken bool
	var oauthToolReverseMap map[string]string

	if req.Body != nil {
		var err error
		originalBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			logrus.WithContext(req.Context()).WithError(err).Errorf("error reading body")
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		modifiedBody = originalBody

		// Check if this is an OAuth token
		key := req.Header.Get("X-Api-Key")
		if key != "" {
			isOAuthToken = IsClaudeOAuthToken(key)
		}

		modifiedBody = applyThinking(modifiedBody)

		// Remap tool names for OAuth tokens to avoid Anthropic fingerprinting
		if isOAuthToken {
			modifiedBody, oauthToolReverseMap = remapOAuthToolNames(modifiedBody)
		}

		// Trim capacity to length to avoid excessive memory usage
		modifiedBody = append([]byte(nil), modifiedBody...)
		// Set GetBody to allow retries and redirects
		req.Body = io.NopCloser(bytes.NewReader(modifiedBody))
		req.ContentLength = int64(len(modifiedBody))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(modifiedBody)), nil
		}
	}

	// Extract model and session ID from request body
	model := extractModelFromBody(originalBody)
	sessionID := extractSessionIDFromBody(originalBody)

	// Set Claude Code specific headers
	t.applyClaudeCodeHeaders(req, isOAuthToken, model, sessionID)

	// Add beta query parameter if not already present
	q := req.URL.Query()
	if !q.Has("beta") {
		q.Add("beta", "true")
		req.URL.RawQuery = q.Encode()
	}

	// Execute the request
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.WithContext(req.Context()).WithError(err).Errorf("failed to round trip request: %v", err)
		return nil, err
	}

	// Restore OAuth tool names in non-streaming responses
	if len(oauthToolReverseMap) > 0 && resp != nil && resp.Body != nil {
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr == nil {
				respBody = reverseRemapOAuthToolNames(respBody, oauthToolReverseMap)
				resp.Body = io.NopCloser(bytes.NewReader(respBody))
				resp.ContentLength = int64(len(respBody))
			}
		}
	}

	return resp, nil
}

func applyThinking(body []byte) []byte {
	res, err := sjson.DeleteBytes(body, "thinking")
	if err != nil {
		logrus.WithError(err).Errorf("error applying thinking")
		return body
	}

	res, err = sjson.SetBytes(res, "output_config", map[string]interface{}{"effort": "medium"})
	if err != nil {
		logrus.WithError(err).Errorf("error applying thinking")
		return body
	}
	return res
}

// applyClaudeCodeHeaders sets all Claude Code specific headers
func (t *claudeRoundTripper) applyClaudeCodeHeaders(req *http.Request, isOAuthToken bool, model string, sessionID string) {
	key := req.Header.Get("X-Api-Key")
	if key == "" {
		return
	}

	// Check if target is Anthropic's API
	isAnthropicBase := req.URL != nil && strings.Contains(strings.ToLower(req.URL.Host), "api.anthropic.com")

	if !isAnthropicBase {
		panic("Impossible to use claude client for server not anthropic")
	}

	if isOAuthToken {
		req.Header.Del("X-Api-Key")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))

	} else {
		req.Header.Del("X-Api-Key")
		req.Header.Set("X-Api-Key", key)
	}

	// Clear and set Claude Code specific headers
	// First, clear headers that may have been set by the SDK
	for k := range req.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-stainless-") ||
			strings.HasPrefix(strings.ToLower(k), "anthropic-") ||
			k == "User-Agent" || k == "X-App" {
			delete(req.Header, k)
		}
	}

	// Build beta header with all required flags
	baseBetas := anthropicBeta

	// Add context-1m for models that support it (Sonnet/Opus, not Haiku)
	//if model != "" && supportsContext1M(model) {
	//	baseBetas = strings.TrimRight(baseBetas, ",") + "," + anthropicContext1m
	//}

	// If user provides custom betas, use them
	// we do never use users' beta headers
	//if val := strings.TrimSpace(req.Header.Get("Anthropic-Beta")); val != "" {
	//	baseBetas = val
	//}

	baseBetas = strings.TrimRight(baseBetas, ",")

	// Ensure oauth is always present at the end
	if !strings.Contains(baseBetas, "oauth") {
		baseBetas = strings.TrimRight(baseBetas, ",")
		baseBetas = fmt.Sprintf("%s,%s", baseBetas, anthropicOAuthBeta)
	}

	// Set all headers via map
	headers := map[string]string{
		"accept":         acceptHeader,
		"anthropic-beta": baseBetas,
		"anthropic-dangerous-direct-browser-access": anthropicDangerousDirectBrowserAccess,
		"anthropic-version":                         anthropicVersion,
		"user-agent":                                claudeCLIUserAgent,
		"x-app":                                     claudeXApp,
		"x-stainless-helper-method":                 stainlessHelperMethod,
		"x-stainless-retry-count":                   stainlessRetryCount,
		"x-stainless-runtime-version":               stainlessRuntimeVersion,
		"x-stainless-package-version":               stainlessPackageVersion,
		"x-stainless-runtime":                       stainlessRuntime,
		"x-stainless-lang":                          stainlessLang,
		"x-stainless-arch":                          stainlessArch(),
		"x-stainless-os":                            stainlessOS(),
		"x-stainless-timeout":                       stainlessTimeout,
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if sessionID != "" {
		req.Header.Set("X-Claude-Code-Session-Id", sessionID)
	}
}
