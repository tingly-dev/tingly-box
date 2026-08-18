package ops

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ApplyProviderTransforms applies provider-specific transformations to an
// OpenAI Chat request. The dispatch matches the provider URL's host (and,
// where a vendor's quirk is scoped to one path on a shared host, a path
// prefix too) — short, explicit, and parallel to the per-shape dispatch in
// VendorTransform.
//
// New providers are added as new cases here; aliases (e.g. multiple URLs that
// share a vendor's quirks) sit in the same case body.
func ApplyProviderTransforms(req *openai.ChatCompletionNewParams, providerURL, model string, config *protocol.OpenAIConfig) *openai.ChatCompletionNewParams {
	host, path := SplitProviderHostPath(providerURL)
	modelLower := strings.ToLower(model)

	// See stripOpenAIPromptCacheFields for why: most OpenAI-compatible
	// vendors reject these fields outright (#1548), so default to stripping.
	nativeOpenAI := supportsExplicitPromptCache(host)
	if !nativeOpenAI {
		stripOpenAIPromptCacheFields(req)
	}

	switch {
	case host == "api.deepseek.com",
		host == "opencode.ai" && strings.HasPrefix(path, "/zen/go") && strings.Contains(modelLower, "deepseek"):
		return applyDeepSeekTransform(req, providerURL, model, config)

	case host == "api.moonshot.cn",
		host == "api.moonshot.ai",
		// api.kimi.com is Moonshot's own dedicated host; the only product
		// catalogued on it today is /coding/v1, and the wire protocol is a
		// property of the vendor/model, not the product path, so a host-only
		// match is enough here (unlike opencode.ai below, a multi-vendor
		// relay where the path is load-bearing).
		host == "api.kimi.com":
		return applyKimiTransform(req, providerURL, model, config)

	case host == "generativelanguage.googleapis.com" && strings.Contains(modelLower, "gemini"):
		return applyGeminiTransform(req, providerURL, model, config)

	case host == "poe.com" && strings.Contains(modelLower, "gemini"):
		return applyGeminiPoeTransform(req, providerURL, model, config)
	}

	// api.openai.com falls through to here too — no vendor-specific shaping
	// needed beyond applyDefaultTransform's thinking fallback.
	return applyDefaultTransform(req, config, nativeOpenAI)
}

// supportsExplicitPromptCache reports whether the provider host is confirmed
// to accept OpenAI's gpt-5.6+ explicit prompt-cache fields. Extend this
// allowlist only once a vendor has been verified to accept the fields —
// the default (stripped) is the safe outcome for an unverified vendor.
func supportsExplicitPromptCache(host string) bool {
	return host == "api.openai.com"
}

// stripOpenAIPromptCacheFields removes the OpenAI-only prompt-cache fields
// from a request: top-level prompt_cache_options and prompt_cache_retention,
// and the per-content-part prompt_cache_breakpoint markers. It's the default
// for every vendor not on the supportsExplicitPromptCache allowlist — most
// OpenAI-compatible vendors don't implement these fields and strict-schema
// gateways reject the whole request over the unknown one (NVIDIA NIM 400s on
// the top-level fields, #1548). Dropping them is safe: they're pure caching
// hints, and vendors with their own automatic prefix caching (DeepSeek,
// Moonshot, most self-hosted backends) still get cache hits without them.
//
// Scope is exactly these three fields, per the SDK's history (libs/openai-go):
// prompt_cache_retention shipped with gpt-5.1, prompt_cache_options /
// prompt_cache_breakpoint with gpt-5.6. prompt_cache_key predates both by
// over a year (SDK v1.12.0), is part of the schema every OpenAI-compatible
// vendor cloned, and is deliberately left alone.
//
// All three carry omitzero, so zeroing them omits the keys from the
// marshaled request without a JSON round-trip — which would drop per-message
// extra fields such as x_thinking / reasoning_content.
func stripOpenAIPromptCacheFields(req *openai.ChatCompletionNewParams) {
	req.PromptCacheOptions = openai.ChatCompletionNewParamsPromptCacheOptions{}
	req.PromptCacheRetention = ""

	for i := range req.Messages {
		msg := &req.Messages[i]
		switch {
		case msg.OfDeveloper != nil:
			stripTextPartBreakpoints(msg.OfDeveloper.Content.OfArrayOfContentParts)
		case msg.OfSystem != nil:
			stripTextPartBreakpoints(msg.OfSystem.Content.OfArrayOfContentParts)
		case msg.OfUser != nil:
			for j := range msg.OfUser.Content.OfArrayOfContentParts {
				part := &msg.OfUser.Content.OfArrayOfContentParts[j]
				switch {
				case part.OfText != nil:
					part.OfText.PromptCacheBreakpoint = openai.ChatCompletionContentPartTextPromptCacheBreakpointParam{}
				case part.OfImageURL != nil:
					part.OfImageURL.PromptCacheBreakpoint = openai.ChatCompletionContentPartImagePromptCacheBreakpointParam{}
				case part.OfInputAudio != nil:
					part.OfInputAudio.PromptCacheBreakpoint = openai.ChatCompletionContentPartInputAudioPromptCacheBreakpointParam{}
				case part.OfFile != nil:
					part.OfFile.PromptCacheBreakpoint = openai.ChatCompletionContentPartFilePromptCacheBreakpointParam{}
				}
			}
		case msg.OfAssistant != nil:
			for j := range msg.OfAssistant.Content.OfArrayOfContentParts {
				part := &msg.OfAssistant.Content.OfArrayOfContentParts[j]
				if part.OfText != nil {
					part.OfText.PromptCacheBreakpoint = openai.ChatCompletionContentPartTextPromptCacheBreakpointParam{}
				}
			}
		case msg.OfTool != nil:
			stripTextPartBreakpoints(msg.OfTool.Content.OfArrayOfContentParts)
		}
	}
}

func stripTextPartBreakpoints(parts []openai.ChatCompletionContentPartTextParam) {
	for i := range parts {
		parts[i].PromptCacheBreakpoint = openai.ChatCompletionContentPartTextPromptCacheBreakpointParam{}
	}
}

// ApplyCursorCompatContentNormalization flattens rich content in messages for
// Cursor compatibility. Applies to ALL providers when cursor_compat is enabled.
func ApplyCursorCompatContentNormalization(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		msgMap, err := messageToMap(req.Messages[i])
		if err != nil {
			continue
		}
		content, ok := msgMap["content"]
		if !ok {
			continue
		}
		contentParts, ok := content.([]interface{})
		if !ok {
			continue
		}
		flattened, _ := flattenRichContent(contentParts)
		msgMap["content"] = flattened

		updatedBytes, err := json.Marshal(msgMap)
		if err != nil {
			continue
		}
		var updated openai.ChatCompletionMessageParamUnion
		if err := json.Unmarshal(updatedBytes, &updated); err != nil {
			continue
		}
		req.Messages[i] = updated
	}
}

func messageToMap(msg openai.ChatCompletionMessageParamUnion) (map[string]interface{}, error) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(msgBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func flattenRichContent(parts []interface{}) (string, bool) {
	var segments []string
	var dropped bool
	for _, part := range parts {
		switch value := part.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				segments = append(segments, value)
			}
		case map[string]interface{}:
			if textValue, ok := value["text"].(string); ok {
				if strings.TrimSpace(textValue) != "" {
					segments = append(segments, textValue)
				}
			} else if contentValue, ok := value["content"].(string); ok {
				if strings.TrimSpace(contentValue) != "" {
					segments = append(segments, contentValue)
				}
			} else {
				dropped = true
			}
		default:
			dropped = true
		}
	}
	if len(segments) == 0 && dropped {
		return "[non-text content omitted]", true
	}
	if dropped {
		segments = append(segments, "[non-text content omitted]")
	}
	return strings.Join(segments, "\n"), dropped
}

// applyDefaultTransform applies the standard OpenAI-compatible thinking
// fallback when no vendor-specific transform matched. Sets reasoning_effort
// from config, or falls back to a `thinking.type=enabled` extra field for
// providers that accept the Anthropic-style extension.
//
// nativeReasoningEffort mirrors supportsExplicitPromptCache: only a
// confirmed-OpenAI host gets the full six-level ladder verbatim. Every other
// OpenAI-compatible vendor reached through here — including relays like
// opencode.ai/zen/go whose model name doesn't hint at a specific
// vendor-transform case above — gets the effort collapsed through
// genericEffortTiers instead of "minimal"/"xhigh" sent raw to a vendor that
// has never seen those enum members.
func applyDefaultTransform(req *openai.ChatCompletionNewParams, config *protocol.OpenAIConfig, nativeReasoningEffort bool) *openai.ChatCompletionNewParams {
	if config.HasThinking && config.ReasoningEffort != "" {
		if nativeReasoningEffort {
			req.ReasoningEffort = config.ReasoningEffort
		} else {
			applyReasoningEffortTier(req, config, genericEffortTiers())
		}
	} else if config.HasThinking {
		extra := req.ExtraFields()
		if extra == nil {
			extra = map[string]interface{}{
				"thinking": map[string]interface{}{"type": "enabled"},
			}
		} else {
			extra["thinking"] = map[string]interface{}{"type": "enabled"}
		}
		req.SetExtraFields(extra)
	}
	return req
}
