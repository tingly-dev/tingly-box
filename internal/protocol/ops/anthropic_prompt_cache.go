package ops

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// Anthropic prompt-cache breakpoints (cache_control) are kept only for the
// backends known to implement them: Anthropic itself and Claude OAuth (see
// VendorTransform's isClaudeCodeBackend). Every other Anthropic-compatible
// provider gets the request with all cache_control removed and relies on its
// own automatic prefix caching.
//
// This mirrors the default-deny the OpenAI side already applies
// (supportsExplicitPromptCache, #1554): cache_control is a hint about the
// prefix, not part of it, so dropping it never changes what the model sees —
// but a third-party backend that half-implements it (caching only at the
// marked blocks, or switching its automatic cache off once any marker is
// present) can serve far fewer cached tokens than it would without it.

// StripAnthropicV1CacheControl removes every cache_control from a v1 request:
// top level, system blocks, tool definitions, message content blocks and the
// blocks nested inside tool results.
func StripAnthropicV1CacheControl(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	req.CacheControl = anthropic.CacheControlEphemeralParam{}
	for i := range req.System {
		req.System[i].CacheControl = anthropic.CacheControlEphemeralParam{}
	}
	for i := range req.Tools {
		if cc := req.Tools[i].GetCacheControl(); cc != nil {
			*cc = anthropic.CacheControlEphemeralParam{}
		}
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			block := &req.Messages[i].Content[j]
			if cc := block.GetCacheControl(); cc != nil {
				*cc = anthropic.CacheControlEphemeralParam{}
			}
			if block.OfToolResult != nil {
				for k := range block.OfToolResult.Content {
					if cc := block.OfToolResult.Content[k].GetCacheControl(); cc != nil {
						*cc = anthropic.CacheControlEphemeralParam{}
					}
				}
			}
		}
	}
}

// StripAnthropicBetaCacheControl is StripAnthropicV1CacheControl for beta
// requests.
func StripAnthropicBetaCacheControl(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	req.CacheControl = anthropic.BetaCacheControlEphemeralParam{}
	for i := range req.System {
		req.System[i].CacheControl = anthropic.BetaCacheControlEphemeralParam{}
	}
	for i := range req.Tools {
		if cc := req.Tools[i].GetCacheControl(); cc != nil {
			*cc = anthropic.BetaCacheControlEphemeralParam{}
		}
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			block := &req.Messages[i].Content[j]
			if cc := block.GetCacheControl(); cc != nil {
				*cc = anthropic.BetaCacheControlEphemeralParam{}
			}
			if block.OfToolResult != nil {
				for k := range block.OfToolResult.Content {
					if cc := block.OfToolResult.Content[k].GetCacheControl(); cc != nil {
						*cc = anthropic.BetaCacheControlEphemeralParam{}
					}
				}
			}
		}
	}
}
