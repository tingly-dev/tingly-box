package obs

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ResponseFingerprint is a compact, privacy-safe summary of one provider
// response. It is the passive Tier 0 input to model fingerprinting (see
// .research/model-fingerprinting.md §5/§6): identity signals are derived from
// the response schema, the stream shape and timing — never from message
// content. It is attached to a Record only when the final_response capture
// point is selected.
//
// Fields mirror the fingerprint schema's t0 block closely enough that a
// record can be lifted into a `model-fingerprint/v0.1` document without
// re-reading the body.
type ResponseFingerprint struct {
	// Channel is how the request reached the provider. Passive capture always
	// runs through tingly-box, so this is "through_tb"; direct probes must not
	// be compared with it (model-fingerprinting.md §7.5).
	Channel string `json:"channel,omitempty"`
	// Protocol is the provider API shape, best-effort: "openai", "anthropic",
	// or the provider's APIStyle string.
	Protocol   string `json:"protocol,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`

	// Headers are the response headers, already filtered to a safe allowlist
	// and with id-bearing values reduced to patterns. Provider raw headers are
	// NOT available through the SDK forwarding path (Phase 3 wire recorder);
	// today these are the gateway's own response headers.
	Headers map[string]string `json:"headers,omitempty"`

	// Model is the model string echoed in the response body. Caution: the
	// gateway rewrites this to the response model, so it is not identity
	// evidence on the through_tb channel.
	Model string `json:"model,omitempty"`
	// IDPattern is the response id reduced to a prefix pattern (e.g.
	// "chatcmpl-*", "msg_*"); the raw id is never stored.
	IDPattern string `json:"id_pattern,omitempty"`
	// SystemFingerprint is OpenAI's system_fingerprint (a strong serving-path
	// signal when present).
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
	// ServiceTier is OpenAI's service_tier, when echoed.
	ServiceTier string `json:"service_tier,omitempty"`

	// FinishReasons lists the distinct finish/stop reasons observed.
	FinishReasons []string `json:"finish_reasons,omitempty"`

	// Usage is the normalised usage block plus the set of usage key paths the
	// provider actually emitted (schema signal).
	Usage *UsageFingerprint `json:"usage,omitempty"`

	// Stream is present only for streamed responses.
	Stream *StreamFingerprint `json:"stream,omitempty"`

	ElapsedMs int64 `json:"elapsed_ms,omitempty"`
}

// UsageFingerprint normalises the two common usage shapes (OpenAI
// prompt/completion vs Anthropic input/output) into one integer vector and
// records which key paths were present.
type UsageFingerprint struct {
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// Keys is the sorted set of leaf paths seen under "usage", e.g.
	// "completion_tokens", "prompt_tokens_details.cached_tokens".
	Keys []string `json:"keys,omitempty"`
}

// StreamFingerprint summarises an SSE stream without retaining any content:
// shape, event vocabulary and timing are the fingerprint inputs.
type StreamFingerprint struct {
	ChunkCount int      `json:"chunk_count,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
	// UsageLast reports whether usage first appeared in the final chunk (some
	// providers only send the usage chunk last).
	UsageLast bool `json:"usage_last,omitempty"`
	// DoneMarker reports whether a terminal event (message_stop /
	// response.completed) was observed. The OpenAI chat [DONE] sentinel is
	// consumed by the SSE decoder and is not observable here.
	DoneMarker bool `json:"done_marker,omitempty"`

	// FirstChunkMs is time from request start to the first SSE chunk.
	FirstChunkMs int64 `json:"first_chunk_ms,omitempty"`
	// TTFTMs is time from request start to the first chunk carrying content.
	TTFTMs int64 `json:"ttft_ms,omitempty"`
	// InterChunk* summarise gaps between consecutive chunks (excluding the
	// first). Only aggregate stats are retained.
	InterChunkCount int   `json:"inter_chunk_count,omitempty"`
	InterChunkMinMs int64 `json:"inter_chunk_min_ms,omitempty"`
	InterChunkMaxMs int64 `json:"inter_chunk_max_ms,omitempty"`
	TotalMs         int64 `json:"total_ms,omitempty"`
}

const (
	// DefaultMaxResponseBodyBytes caps the persisted final_response body. The
	// fingerprint is computed before the cap, so a truncated body still yields
	// a complete fingerprint. 1 MiB comfortably fits a normal completion while
	// refusing runaway tool-call/attachment payloads.
	DefaultMaxResponseBodyBytes = 1 << 20

	// maxHeaderValueLen bounds each persisted header value.
	maxHeaderValueLen = 256
	// maxFingerprintHeaders bounds how many headers the fingerprint keeps.
	maxFingerprintHeaders = 32
	// maxFullHeaders bounds the headers kept on the full final_response.
	maxFullHeaders = 64
)

// sensitiveHeaders are never persisted in any response header map.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"api-key":             true,
	"x-goog-api-key":      true,
	"x-auth-token":        true,
}

// fingerprintHeaderAllowlist is the set of response headers kept in the
// fingerprint. It is an allowlist (not a denylist) because headers can carry
// account/request identity. Header matching is case-insensitive.
var fingerprintHeaderAllowlist = map[string]bool{
	"content-type":                   true,
	"content-length":                 true,
	"cache-control":                  true,
	"server":                         true,
	"via":                            true,
	"date":                           true,
	"x-request-id":                   true,
	"request-id":                     true,
	"openai-processing-ms":           true,
	"openai-organization":            true,
	"openai-project":                 true,
	"x-envoy-upstream-service-time":  true,
	"cf-ray":                         true,
	"x-amzn-requestid":               true,
	"x-served-by":                    true,
	"x-cache":                        true,
	"x-timer":                        true,
	"x-ratelimit-limit-requests":     true,
	"x-ratelimit-remaining-requests": true,
	"x-ratelimit-limit-tokens":       true,
	"x-ratelimit-remaining-tokens":   true,
}

// idBearingHeaders are reduced to a prefix pattern instead of the raw value.
var idBearingHeaders = map[string]bool{
	"x-request-id":     true,
	"request-id":       true,
	"cf-ray":           true,
	"x-amzn-requestid": true,
}

// RedactFullHeaders copies headers for persistence on the full final_response:
// sensitive names are dropped, values are truncated, and the map is bounded.
// Unlike FingerprintHeaders it keeps non-sensitive headers (debugging value).
func RedactFullHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[strings.ToLower(k)] {
			continue
		}
		if len(out) >= maxFullHeaders {
			break
		}
		out[k] = truncate(v, maxHeaderValueLen)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FingerprintHeaders returns the allowlisted subset of response headers, with
// id-bearing values reduced to patterns. Never returns raw ids.
func FingerprintHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range h {
		lk := strings.ToLower(k)
		if sensitiveHeaders[lk] || !fingerprintHeaderAllowlist[lk] {
			continue
		}
		if len(out) >= maxFingerprintHeaders {
			break
		}
		if idBearingHeaders[lk] {
			v = IDPattern(v)
		}
		out[lk] = truncate(v, maxHeaderValueLen)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// IDPattern reduces a provider-issued id to a prefix pattern so the raw value
// is not persisted. "chatcmpl-AbC123" -> "chatcmpl-*", "msg_abc" -> "msg_*",
// ids with no separator collapse to "*".
func IDPattern(id string) string {
	if id == "" {
		return ""
	}
	for i, r := range id {
		if r == '-' || r == '_' || r == ':' {
			return id[:i+1] + "*"
		}
	}
	return "*"
}

// BuildResponseFingerprint extracts the passive fingerprint from a response
// body (which may be nil for a streaming passthrough that only has chunks) and
// the response headers. Stream metrics are attached by the caller.
func BuildResponseFingerprint(protocol string, status int, headers map[string]string, body map[string]interface{}, elapsed time.Duration) *ResponseFingerprint {
	fp := &ResponseFingerprint{
		Channel:    "through_tb",
		Protocol:   protocol,
		StatusCode: status,
		Headers:    FingerprintHeaders(headers),
		ElapsedMs:  elapsed.Milliseconds(),
	}
	if body == nil {
		return fp
	}
	fp.Model = asString(body["model"])
	fp.IDPattern = IDPattern(asString(body["id"]))
	fp.SystemFingerprint = asString(body["system_fingerprint"])
	fp.ServiceTier = asString(body["service_tier"])
	fp.FinishReasons = collectFinishReasons(body)
	fp.Usage = ExtractUsage(body)
	return fp
}

// ExtractUsage normalises an OpenAI- or Anthropic-shaped usage block. Returns
// nil when the body has no usage object.
func ExtractUsage(body map[string]interface{}) *UsageFingerprint {
	u, ok := body["usage"].(map[string]interface{})
	if !ok || len(u) == 0 {
		return nil
	}
	fp := &UsageFingerprint{}
	fp.InputTokens = firstInt(u, "input_tokens", "prompt_tokens")
	fp.OutputTokens = firstInt(u, "output_tokens", "completion_tokens")
	fp.CacheReadTokens = firstInt(u, "cache_read_input_tokens")
	if fp.CacheReadTokens == 0 {
		fp.CacheReadTokens = nestedInt(u, "prompt_tokens_details", "cached_tokens")
	}
	fp.CacheWriteTokens = firstInt(u, "cache_creation_input_tokens")
	if fp.CacheWriteTokens == 0 {
		fp.CacheWriteTokens = nestedInt(u, "input_tokens_details", "cache_write_tokens")
	}
	fp.ReasoningTokens = nestedInt(u, "completion_tokens_details", "reasoning_tokens")
	if fp.ReasoningTokens == 0 {
		fp.ReasoningTokens = nestedInt(u, "output_tokens_details", "reasoning_tokens")
	}
	fp.TotalTokens = firstInt(u, "total_tokens")
	fp.Keys = usageLeafKeys(u)
	return fp
}

// CapResponseBody enforces maxBytes on a response body. When the body exceeds
// the cap it is replaced by a compact summary (fingerprint-relevant scalars +
// hash) so the record stays useful without storing user content verbatim.
// maxBytes <= 0 disables the cap.
func CapResponseBody(body map[string]interface{}, maxBytes int) map[string]interface{} {
	if body == nil || maxBytes <= 0 {
		return body
	}
	data, err := json.Marshal(body)
	if err != nil || len(data) <= maxBytes {
		return body
	}
	sum := map[string]interface{}{
		"_truncated": true,
		"_bytes":     len(data),
		"_sha256":    hashBytes(data),
	}
	for _, k := range []string{"id", "model", "system_fingerprint", "service_tier", "usage"} {
		if v, ok := body[k]; ok {
			sum[k] = v
		}
	}
	return sum
}

// collectFinishReasons gathers distinct finish/stop reasons from the common
// response shapes (OpenAI choices[].finish_reason, Anthropic stop_reason,
// Responses status/incomplete_details).
func collectFinishReasons(body map[string]interface{}) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if choices, ok := body["choices"].([]interface{}); ok {
		for _, c := range choices {
			if m, ok := c.(map[string]interface{}); ok {
				add(asString(m["finish_reason"]))
			}
		}
	}
	add(asString(body["stop_reason"]))
	if details, ok := body["incomplete_details"].(map[string]interface{}); ok {
		if s := asString(details["reason"]); s != "" {
			add("incomplete:" + s)
		}
	}
	if status := asString(body["status"]); status == "incomplete" {
		add("incomplete")
	}
	return out
}

// usageLeafKeys returns the sorted leaf key paths under a usage map.
func usageLeafKeys(u map[string]interface{}) []string {
	var keys []string
	var walk func(prefix string, m map[string]interface{})
	walk = func(prefix string, m map[string]interface{}) {
		for k, v := range m {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if child, ok := v.(map[string]interface{}); ok {
				walk(path, child)
				continue
			}
			keys = append(keys, path)
		}
	}
	walk("", u)
	sort.Strings(keys)
	return keys
}

func firstInt(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if n, ok := toInt(m[k]); ok {
			return n
		}
	}
	return 0
}

func nestedInt(m map[string]interface{}, parent, child string) int {
	if p, ok := m[parent].(map[string]interface{}); ok {
		if n, ok := toInt(p[child]); ok {
			return n
		}
	}
	return 0
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
