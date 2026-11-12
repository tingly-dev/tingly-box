package ops

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
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
	if !acceptsChatArrayTextContent(host) {
		compactOpenAIChatTextContent(req)
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

// acceptsChatArrayTextContent reports whether the provider host is confirmed to
// accept the content-part array form for text-only system, user, assistant and
// tool messages.
//
// Kept separate from supportsExplicitPromptCache, which holds the same single
// entry today: "accepts the prompt-cache fields" and "accepts array text
// content" are different questions, and one allowlist answering both would
// silently flip a vendor's entire text wire format the day it is added for the
// cache fields alone.
//
// They are not independent in both directions, though. A prompt-cache
// breakpoint is a field on a content *part*, so it can only reach a vendor that
// takes the array form — an allowlisted-for-cache host that was left off this
// list would have its breakpoints compacted away without a word. The
// implication is therefore encoded here rather than left to whoever edits the
// lists next: array content is a prerequisite for the cache fields, never the
// other way round.
func acceptsChatArrayTextContent(host string) bool {
	return host == "api.openai.com" || supportsExplicitPromptCache(host)
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
			for j := range msg.OfTool.Content.OfArrayOfContentParts {
				part := &msg.OfTool.Content.OfArrayOfContentParts[j]
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
		}
	}
}

func stripTextPartBreakpoints(parts []openai.ChatCompletionContentPartTextParam) {
	for i := range parts {
		parts[i].PromptCacheBreakpoint = openai.ChatCompletionContentPartTextPromptCacheBreakpointParam{}
	}
}

// compactOpenAIChatTextContent collapses all-text content-part arrays back to
// the plain string form, for every vendor not on the acceptsChatArrayTextContent
// allowlist.
//
// The converters emit the content-part list unconditionally, because letting a
// cache breakpoint decide an item's shape invalidates the upstream prompt cache
// the turn a client rolls that breakpoint forward (see the cache-shape
// invariant in internal/protocol/request/cache_control.go). That is the right
// shape to carry *through* the gateway, but it is the richer of the two wire
// forms, and an OpenAI-compatible vendor that only accepts a string for system,
// assistant or tool content would reject every request.
//
// Nothing in the array form is lost by collapsing it: its only extra
// expressiveness over a string is the prompt-cache breakpoints, and a vendor
// reaching this point is by construction one whose breakpoints were stripped
// just above (see acceptsChatArrayTextContent). Both branches stay
// shape-stable — an off-allowlist vendor always sees strings, an allowlisted one
// always sees parts — and neither depends on where a breakpoint sat. Parts are
// joined without a separator, matching the concatenation the converters did
// before the arrays became unconditional. Content holding anything but text
// (images, audio, files) keeps the array, since the string form cannot express
// it.
func compactOpenAIChatTextContent(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		msg := &req.Messages[i]
		switch {
		case msg.OfDeveloper != nil:
			compactParts(&msg.OfDeveloper.Content.OfString, &msg.OfDeveloper.Content.OfArrayOfContentParts, textPartText)
		case msg.OfSystem != nil:
			compactParts(&msg.OfSystem.Content.OfString, &msg.OfSystem.Content.OfArrayOfContentParts, textPartText)
		case msg.OfUser != nil:
			compactParts(&msg.OfUser.Content.OfString, &msg.OfUser.Content.OfArrayOfContentParts, unionPartText)
		case msg.OfTool != nil:
			compactParts(&msg.OfTool.Content.OfString, &msg.OfTool.Content.OfArrayOfContentParts, unionPartText)
		case msg.OfAssistant != nil:
			compactParts(&msg.OfAssistant.Content.OfString, &msg.OfAssistant.Content.OfArrayOfContentParts, assistantPartText)
		}
	}
}

// compactParts replaces an all-text part list with the equivalent string.
// text reports a part's text and whether the part is text at all; one part that
// is not (an image, audio, a refusal) leaves the list untouched, and the scan
// for that happens before any copying so a long text prefix ahead of an image
// is never copied just to be discarded.
func compactParts[T any](str *param.Opt[string], parts *[]T, text func(T) (string, bool)) {
	if len(*parts) == 0 {
		return // already a string (or genuinely absent) — nothing to collapse
	}
	total := 0
	for _, part := range *parts {
		s, ok := text(part)
		if !ok {
			return
		}
		total += len(s)
	}
	// The overwhelmingly common case: one text part, whose string is reused
	// as-is rather than copied through a builder. A request carries one of
	// these per message.
	if len(*parts) == 1 {
		s, _ := text((*parts)[0])
		*parts = nil
		*str = openai.String(s)
		return
	}
	var joined strings.Builder
	joined.Grow(total)
	for _, part := range *parts {
		s, _ := text(part)
		joined.WriteString(s)
	}
	*parts = nil
	*str = openai.String(joined.String())
}

func textPartText(part openai.ChatCompletionContentPartTextParam) (string, bool) {
	return part.Text, true
}

func unionPartText(part openai.ChatCompletionContentPartUnionParam) (string, bool) {
	if part.OfText == nil {
		return "", false
	}
	return part.OfText.Text, true
}

func assistantPartText(part openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion) (string, bool) {
	if part.OfText == nil {
		return "", false // a refusal part has no string form
	}
	return part.OfText.Text, true
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
// confirmed-OpenAI host gets the six-level ladder verbatim; everything else
// collapses through genericEffortTiers (see .design/model-data.md).
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
