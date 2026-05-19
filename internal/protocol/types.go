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

	TypeOpenAIChat      APIType = publicprotocol.TypeOpenAIChat
	TypeOpenAIResponses APIType = publicprotocol.TypeOpenAIResponses
	TypeAnthropicV1     APIType = publicprotocol.TypeAnthropicV1
	TypeAnthropicBeta   APIType = publicprotocol.TypeAnthropicBeta
	TypeGoogle          APIType = publicprotocol.TypeGoogle

	CodexAPIBase = publicprotocol.CodexAPIBase
)

// Re-export functions for backward compatibility
var (
	NewTokenUsage          = publicprotocol.NewTokenUsage
	NewTokenUsageWithCache = publicprotocol.NewTokenUsageWithCache
	NewTokenUsageFull      = publicprotocol.NewTokenUsageFull
	ZeroTokenUsage         = publicprotocol.ZeroTokenUsage
)
