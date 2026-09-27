// Package protocol provides backward compatibility aliases to the public protocol package.
// All code should migrate to use "github.com/tingly-dev/tingly-box/protocol" directly.
package protocol

import (
	publicprotocol "github.com/tingly-dev/tingly-box/ai"
)

// Type aliases to public protocol types for backward compatibility
type APIStyle = publicprotocol.APIStyle
type APIType = publicprotocol.APIType
type Client = publicprotocol.Client
type TokenUsage = publicprotocol.TokenUsage
type OpenAIConfig = publicprotocol.OpenAIConfig

// Re-export constants for backward compatibility
const (
	APIStyleOpenAI    APIStyle = publicprotocol.APIStyleOpenAI
	APIStyleAnthropic APIStyle = publicprotocol.APIStyleAnthropic
	APIStyleGoogle    APIStyle = publicprotocol.APIStyleGoogle
	APIStyleDecision  APIStyle = publicprotocol.APIStyleDecision

	TypeOpenAIChat      APIType = publicprotocol.TypeOpenAIChat
	TypeOpenAIResponses APIType = publicprotocol.TypeOpenAIResponses
	TypeAnthropicV1     APIType = publicprotocol.TypeAnthropicV1
	TypeAnthropicBeta   APIType = publicprotocol.TypeAnthropicBeta
	TypeGoogle          APIType = publicprotocol.TypeGoogle
	TypeDecision        APIType = publicprotocol.TypeDecision

	CodexAPIBase = publicprotocol.CodexAPIBase
)

// Re-export functions for backward compatibility
var (
	NewTokenUsage          = publicprotocol.NewTokenUsage
	NewTokenUsageWithCache = publicprotocol.NewTokenUsageWithCache
	NewTokenUsageFull      = publicprotocol.NewTokenUsageFull
	ZeroTokenUsage         = publicprotocol.ZeroTokenUsage
)

// PromptCacheHintFields names every field that is a prompt-cache *hint* rather
// than part of the prompt itself, in both protocol families' spellings:
// Anthropic's cache_control and OpenAI's prompt_cache_* family.
//
// A prompt cache is keyed on the request prefix, and these fields describe that
// prefix rather than belong to it, so removing them is what "compare what the
// upstream actually caches" means. The list is declared once here because three
// places need the same answer: the vendor-boundary strip in
// internal/protocol/ops, and the two prefix-stability suites that diff request
// bodies across turns. A field added to the SDK and missed by one of them would
// make that suite quietly test something weaker.
var PromptCacheHintFields = []string{
	"cache_control",
	"prompt_cache_breakpoint",
	"prompt_cache_options",
	"prompt_cache_retention",
	"prompt_cache_key",
}
