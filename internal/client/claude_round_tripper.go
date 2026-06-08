package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// knownAnthropicBetas is the universe of valid anthropic-beta flag values
// the upstream API recognizes — sourced from the SDK's AnthropicBeta*
// constants plus Claude Code specific flags Anthropic ships ahead of the
// public SDK. Anything outside this set is treated as garbage at the
// outermost layer (likely a buggy or hostile caller).
var knownAnthropicBetas = func() map[string]struct{} {
	flags := []string{
		// SDK-defined (libs/anthropic-sdk-go/beta.go)
		anthropic.AnthropicBetaMessageBatches2024_09_24,
		anthropic.AnthropicBetaPromptCaching2024_07_31,
		anthropic.AnthropicBetaComputerUse2024_10_22,
		anthropic.AnthropicBetaComputerUse2025_01_24,
		anthropic.AnthropicBetaPDFs2024_09_25,
		anthropic.AnthropicBetaTokenCounting2024_11_01,
		anthropic.AnthropicBetaTokenEfficientTools2025_02_19,
		anthropic.AnthropicBetaOutput128k2025_02_19,
		anthropic.AnthropicBetaFilesAPI2025_04_14,
		anthropic.AnthropicBetaMCPClient2025_04_04,
		anthropic.AnthropicBetaMCPClient2025_11_20,
		anthropic.AnthropicBetaDevFullThinking2025_05_14,
		anthropic.AnthropicBetaInterleavedThinking2025_05_14,
		anthropic.AnthropicBetaCodeExecution2025_05_22,
		anthropic.AnthropicBetaExtendedCacheTTL2025_04_11,
		anthropic.AnthropicBetaContext1m2025_08_07,
		anthropic.AnthropicBetaContextManagement2025_06_27,
		anthropic.AnthropicBetaModelContextWindowExceeded2025_08_26,
		anthropic.AnthropicBetaSkills2025_10_02,
		anthropic.AnthropicBetaFastMode2026_02_01,
		anthropic.AnthropicBetaOutput300k2026_03_24,
		anthropic.AnthropicBetaUserProfiles2026_03_24,
		anthropic.AnthropicBetaAdvisorTool2026_03_01,
		anthropic.AnthropicBetaManagedAgents2026_04_01,
		anthropic.AnthropicBetaCacheDiagnosis2026_04_07,
		anthropic.AnthropicBetaThinkingTokenCount2026_05_13,
		// Claude Code specific flags not (yet) exposed by the SDK
		"claude-code-20250219",
		"oauth-2025-04-20",
		"prompt-caching-scope-2026-01-05",
		"structured-outputs-2025-12-15",
		"redact-thinking-2026-02-12",
		"token-efficient-tools-2026-03-28",
		"oidc-federation-2026-04-01",
	}
	m := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		m[f] = struct{}{}
	}
	return m
}()

// claudeCodeRequiredBetasOrdered is the required baseline as an ordered
// slice, derived once from anthropicBeta. mergeBetaFlags iterates this on
// every request instead of re-splitting the constant per call.
var claudeCodeRequiredBetasOrdered = func() []string {
	parts := strings.Split(anthropicBeta, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}()

// claudeCodeRequiredBetas is the set form of the same baseline, for O(1)
// membership checks in classifyUpstreamBetaFlag.
var claudeCodeRequiredBetas = func() map[string]struct{} {
	m := make(map[string]struct{}, len(claudeCodeRequiredBetasOrdered))
	for _, p := range claudeCodeRequiredBetasOrdered {
		m[p] = struct{}{}
	}
	return m
}()

// claudeCodeAllowedUpstreamBetas is the very narrow set of anthropic-beta
// flags we accept FROM upstream callers on top of the required baseline.
//
// Why this is restrictive: Anthropic fingerprints Claude Code OAuth
// traffic, and `anthropic-beta` is one of the signals. Forwarding any
// SDK-known flag (e.g. message-batches, managed-agents, pdfs, mcp-client)
// would emit a header shape no real claude-cli ever sends, which both
// breaks fingerprinting and may trigger anti-abuse responses.
//
// Only flags that real Claude Code is known to add conditionally — or
// that have been explicitly cleared as fingerprint-safe — belong here.
// When in doubt, leave it out.
var claudeCodeAllowedUpstreamBetas = map[string]struct{}{
	// Model-conditional 1M context window; real Claude Code adds this
	// for sonnet/opus, so accepting it from upstream is safe.
	anthropic.AnthropicBetaContext1m2025_08_07: {},
}

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

// classifyUpstreamBetaFlag inspects a token from the upstream
// anthropic-beta header and returns whether to keep it plus a reason
// suitable for logging when it's dropped.
//   - "unknown": not in knownAnthropicBetas — likely garbage or a flag
//     newer than this build. Outer-layer drop.
//   - "not-fingerprint-safe": valid Anthropic flag, but forwarding it
//     would break the claude-cli fingerprint. Inner-layer drop.
//   - "" with keep=true: safe to forward.
func classifyUpstreamBetaFlag(s string) (keep bool, reason string) {
	if _, ok := knownAnthropicBetas[s]; !ok {
		return false, "unknown"
	}
	if _, ok := claudeCodeRequiredBetas[s]; ok {
		return true, ""
	}
	if _, ok := claudeCodeAllowedUpstreamBetas[s]; ok {
		return true, ""
	}
	return false, "not-fingerprint-safe"
}

// mergeBetaFlags builds the outgoing anthropic-beta value in a single pass:
// tokenize → validate → dedupe. The required Claude Code baseline goes
// first (preserving the claude-cli fingerprint), then any upstream-supplied
// flags that are on the narrow allowlist of fingerprint-safe additions.
// Everything else from upstream is dropped with a warn log — see
// claudeCodeAllowedUpstreamBetas. Finally requiredOAuth is appended as a
// fallback if no oauth flag is present in the merged set.
//
// `required` is passed in pre-tokenized so the production caller can hand
// us the package-level slice (claudeCodeRequiredBetasOrdered) and avoid
// re-splitting the constant on every request; tests construct their own.
func mergeBetaFlags(required []string, upstream []string, requiredOAuth string) string {
	seen := make(map[string]struct{})
	var out []string
	hasOAuth := false
	emit := func(p string) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
		if strings.HasPrefix(p, "oauth") {
			hasOAuth = true
		}
	}
	// Required baseline — trusted, emitted as-is.
	for _, p := range required {
		emit(p)
	}
	// Upstream — gated by the fingerprint-safe allowlist.
	for _, v := range upstream {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			keep, reason := classifyUpstreamBetaFlag(p)
			if !keep {
				logrus.WithFields(logrus.Fields{"flag": p, "reason": reason}).
					Warn("dropping upstream anthropic-beta flag")
				continue
			}
			emit(p)
		}
	}
	if !hasOAuth && requiredOAuth != "" {
		emit(strings.TrimSpace(requiredOAuth))
	}
	return strings.Join(out, ",")
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

	// Capture upstream anthropic-beta values BEFORE clearing headers so we can
	// merge any extra flags the caller relies on (e.g. mcp-client-*, cache-*).
	// Dropping them blindly is risky — only well-known Claude Code flags are
	// guaranteed here, anything else the upstream SDK added should pass through.
	upstreamBetas := req.Header.Values("Anthropic-Beta")

	// Clear and set Claude Code specific headers
	// First, clear headers that may have been set by the SDK
	for k := range req.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-stainless-") ||
			strings.HasPrefix(strings.ToLower(k), "anthropic-") ||
			k == "User-Agent" || k == "X-App" {
			delete(req.Header, k)
		}
	}

	// Build beta header: start with the required Claude Code flags, then
	// append any upstream-supplied flags we don't already cover (deduped,
	// order preserved). Required flags stay at the front so the Claude
	// Code fingerprint matches.
	baseBetas := mergeBetaFlags(claudeCodeRequiredBetasOrdered, upstreamBetas, anthropicOAuthBeta)

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
