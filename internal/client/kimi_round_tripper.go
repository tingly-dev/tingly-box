package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Kimi-cli impersonation values sent with every inference request.
// Reference: CLIProxyAPI internal/runtime/executor/kimi_executor.go.
const (
	kimiCLIUserAgent = "KimiCLI/1.10.6"
	kimiCLIPlatform  = "kimi_cli"
	kimiCLIVersion   = "1.10.6"
)

// kimiRoundTripper layers kimi-cli impersonation headers on an inner
// transport. The Authorization Bearer is set by the OpenAI SDK.
//
// All header values are bound at construction so per-request RoundTrip stays
// allocation-free: device id (per-credential, persisted in OAuthDetail.DeviceID),
// hostname (one os.Hostname syscall), and the GOOS/GOARCH-derived model.
type kimiRoundTripper struct {
	http.RoundTripper
	deviceID    string
	deviceName  string
	deviceModel string
	osVersion   string
}

func newKimiRoundTripper(inner http.RoundTripper, provider *typ.Provider) *kimiRoundTripper {
	var deviceID string
	if provider != nil && provider.OAuthDetail != nil {
		deviceID = provider.OAuthDetail.DeviceID
	}
	return &kimiRoundTripper{
		RoundTripper: inner,
		deviceID:     deviceID,
		deviceName:   oauth.KimiDeviceName(),
		deviceModel:  oauth.KimiDeviceModel(),
		osVersion:    oauth.KimiOsVersion(),
	}
}

func (t *kimiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", kimiCLIUserAgent)
	req.Header.Set("X-Msh-Platform", kimiCLIPlatform)
	req.Header.Set("X-Msh-Version", kimiCLIVersion)
	req.Header.Set("X-Msh-Device-Name", t.deviceName)
	req.Header.Set("X-Msh-Device-Model", t.deviceModel)
	req.Header.Set("X-Msh-Os-Version", t.osVersion)
	if t.deviceID != "" {
		req.Header.Set("X-Msh-Device-Id", t.deviceID)
	}

	// Normalize request body for Kimi API compatibility
	// This includes stripping model name prefix and normalizing tool messages
	if req.Body != nil && req.Method == "POST" {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
			normalized, normErr := NormalizeRequest(bodyBytes)
			if normErr == nil {
				req.Body = io.NopCloser(bytes.NewReader(normalized))
				req.ContentLength = int64(len(normalized))
			}
			// If normalization fails, proceed with original body
		}
	}

	return t.RoundTripper.RoundTrip(req)
}

// NormalizeRequest applies Kimi-specific normalization to API request bodies.
// This includes:
// 1. Stripping the "kimi-" prefix from model names
// 2. Normalizing tool message format for Kimi API compatibility
// Reference: CLIProxyAPI internal/runtime/executor/kimi_executor.go
func NormalizeRequest(body []byte) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil
	}

	// Step 1: Strip kimi- prefix from model name
	body = stripKimiPrefixFromBody(body)

	// Step 2: Normalize tool messages
	normalized, err := normalizeToolMessages(body)
	if err != nil {
		return body, fmt.Errorf("failed to normalize tool messages: %w", err)
	}

	return normalized, nil
}

// stripKimiPrefixFromBody removes the "kimi-" prefix from the model field.
// Reference: CLIProxyAPI kimi_executor.go:742-749
// Example: "kimi-k2" → "k2", "Kimi-K2" → "Kimi-K2" (case-sensitive)
func stripKimiPrefixFromBody(body []byte) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}

	model := gjson.GetBytes(body, "model")
	if !model.Exists() {
		return body
	}

	modelName := model.String()
	stripped := stripKimiPrefix(modelName)
	if stripped != modelName {
		// Model name was modified, update the body
		updated, err := sjson.SetBytes(body, "model", stripped)
		if err != nil {
			return body // Return original on error
		}
		return updated
	}

	return body
}

// stripKimiPrefix removes the "kimi-" prefix from model names.
// The prefix check is case-insensitive, but the replacement preserves
// the original case of the remaining part.
// Reference: CLIProxyAPI kimi_executor.go:742-749
func stripKimiPrefix(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(strings.ToLower(model), "kimi-") {
		return model[5:] // Remove "kimi-" prefix (5 characters)
	}
	return model
}

// normalizeToolMessages normalizes tool message format for Kimi API compatibility.
// This includes:
// 1. Filtering empty assistant messages
// 2. Adding reasoning_content to tool call messages
// 3. Fixing tool_call_id vs call_id field names
// Reference: CLIProxyAPI kimi_executor.go:326-449
func normalizeToolMessages(body []byte) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body, nil
	}

	msgs := messages.Array()

	// Step 1: Filter empty assistant messages
	out, dropped, err := filterEmptyAssistantMessages(body, msgs)
	if err != nil {
		return body, err
	}
	if dropped > 0 {
		// Re-read messages after filtering
		messages = gjson.GetBytes(out, "messages")
		if !messages.Exists() || !messages.IsArray() {
			return out, nil
		}
		msgs = messages.Array()
	}

	// Step 2: Normalize tool message fields
	pending := make([]string, 0)
	patched := 0
	patchedReasoning := 0
	latestReasoning := ""
	hasLatestReasoning := false

	removePending := func(id string) {
		for idx := range pending {
			if pending[idx] != id {
				continue
			}
			pending = append(pending[:idx], pending[idx+1:]...)
			return
		}
	}

	for msgIdx := range msgs {
		msg := msgs[msgIdx]
		role := strings.TrimSpace(msg.Get("role").String())

		switch role {
		case "assistant":
			// Track reasoning content for later use
			reasoning := msg.Get("reasoning_content")
			if reasoning.Exists() {
				reasoningText := reasoning.String()
				if strings.TrimSpace(reasoningText) != "" {
					latestReasoning = reasoningText
					hasLatestReasoning = true
				}
			}

			// Track pending tool calls
			toolCalls := msg.Get("tool_calls")
			if !toolCalls.Exists() || !toolCalls.IsArray() || len(toolCalls.Array()) == 0 {
				continue
			}

			// Add reasoning_content if missing
			if !reasoning.Exists() || strings.TrimSpace(reasoning.String()) == "" {
				reasoningText := fallbackAssistantReasoning(msg, hasLatestReasoning, latestReasoning)
				path := fmt.Sprintf("messages.%d.reasoning_content", msgIdx)
				next, err := sjson.SetBytes(out, path, reasoningText)
				if err != nil {
					return body, fmt.Errorf("failed to set assistant reasoning_content: %w", err)
				}
				out = next
				patchedReasoning++
			}

			// Collect tool call IDs
			for _, tc := range toolCalls.Array() {
				id := strings.TrimSpace(tc.Get("id").String())
				if id != "" {
					pending = append(pending, id)
				}
			}

		case "tool":
			// Fix tool_call_id vs call_id field names
			toolCallID := strings.TrimSpace(msg.Get("tool_call_id").String())
			if toolCallID == "" {
				toolCallID = strings.TrimSpace(msg.Get("call_id").String())
				if toolCallID != "" {
					// Fix: rename call_id to tool_call_id
					path := fmt.Sprintf("messages.%d.tool_call_id", msgIdx)
					next, err := sjson.SetBytes(out, path, toolCallID)
					if err != nil {
						return body, fmt.Errorf("failed to set tool_call_id from call_id: %w", err)
					}
					out = next
					patched++
				}
			}

			// Infer missing tool_call_id from pending tool calls
			if toolCallID == "" {
				if len(pending) == 1 {
					toolCallID = pending[0]
					path := fmt.Sprintf("messages.%d.tool_call_id", msgIdx)
					next, err := sjson.SetBytes(out, path, toolCallID)
					if err != nil {
						return body, fmt.Errorf("failed to infer tool_call_id: %w", err)
					}
					out = next
					patched++
				}
				// Note: If len(pending) > 1, we can't infer which ID to use
				// This case is logged but not an error
			}

			// Remove used tool call ID from pending list
			if toolCallID != "" {
				removePending(toolCallID)
			}
		}
	}

	return out, nil
}

// filterEmptyAssistantMessages removes empty assistant messages that don't contain
// tool calls, function calls, reasoning, or meaningful content.
// Reference: CLIProxyAPI kimi_executor.go:451-471
func filterEmptyAssistantMessages(body []byte, msgs []gjson.Result) ([]byte, int, error) {
	kept := make([]string, 0, len(msgs))
	dropped := 0

	for _, msg := range msgs {
		if shouldDropAssistantMessage(msg) {
			dropped++
			continue
		}
		kept = append(kept, msg.Raw)
	}

	if dropped == 0 {
		return body, 0, nil
	}

	rawMessages := []byte("[" + strings.Join(kept, ",") + "]")
	out, err := sjson.SetRawBytes(body, "messages", rawMessages)
	if err != nil {
		return body, 0, fmt.Errorf("failed to drop empty assistant messages: %w", err)
	}

	return out, dropped, nil
}

// shouldDropAssistantMessage determines if an assistant message should be dropped.
// Reference: CLIProxyAPI kimi_executor.go:473-481
func shouldDropAssistantMessage(msg gjson.Result) bool {
	if strings.TrimSpace(msg.Get("role").String()) != "assistant" {
		return false
	}

	if hasToolCalls(msg) || hasLegacyFunctionCall(msg) || hasAssistantReasoning(msg) {
		return false
	}

	return isAssistantContentEmpty(msg.Get("content"))
}

// hasToolCalls checks if message has tool_calls array with items.
func hasToolCalls(msg gjson.Result) bool {
	toolCalls := msg.Get("tool_calls")
	return toolCalls.Exists() && toolCalls.IsArray() && len(toolCalls.Array()) > 0
}

// hasLegacyFunctionCall checks if message has a non-empty function_call.
func hasLegacyFunctionCall(msg gjson.Result) bool {
	functionCall := msg.Get("function_call")
	if !functionCall.Exists() || functionCall.Type == gjson.Null {
		return false
	}
	if functionCall.IsObject() && strings.TrimSpace(functionCall.Raw) == "{}" {
		return false
	}
	return strings.TrimSpace(functionCall.Raw) != ""
}

// hasAssistantReasoning checks if message has reasoning_content.
func hasAssistantReasoning(msg gjson.Result) bool {
	reasoning := msg.Get("reasoning_content")
	return reasoning.Exists() && strings.TrimSpace(reasoning.String()) != ""
}

// isAssistantContentEmpty checks if content field is empty or null.
func isAssistantContentEmpty(content gjson.Result) bool {
	if !content.Exists() || content.Type == gjson.Null {
		return true
	}
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String()) == ""
	}
	if !content.IsArray() {
		return false
	}
	for _, part := range content.Array() {
		if !isAssistantContentPartEmpty(part) {
			return false
		}
	}
	return true
}

// isAssistantContentPartEmpty checks if a content part is empty.
func isAssistantContentPartEmpty(part gjson.Result) bool {
	if !part.Exists() || part.Type == gjson.Null {
		return true
	}
	if part.Type == gjson.String {
		return strings.TrimSpace(part.String()) == ""
	}
	if !part.IsObject() {
		return false
	}
	if text := part.Get("text"); text.Exists() {
		return strings.TrimSpace(text.String()) == ""
	}
	if strings.TrimSpace(part.Get("type").String()) == "text" {
		return true
	}
	return strings.TrimSpace(part.Raw) == "{}"
}

// fallbackAssistantReasoning provides reasoning content when missing.
// Reference: CLIProxyAPI kimi_executor.go:541-567
func fallbackAssistantReasoning(msg gjson.Result, hasLatest bool, latest string) string {
	if hasLatest && strings.TrimSpace(latest) != "" {
		return latest
	}

	content := msg.Get("content")
	if content.Type == gjson.String {
		if text := strings.TrimSpace(content.String()); text != "" {
			return text
		}
	}
	if content.IsArray() {
		parts := make([]string, 0, len(content.Array()))
		for _, item := range content.Array() {
			text := strings.TrimSpace(item.Get("text").String())
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}

	return ""
}
