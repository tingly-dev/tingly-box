// Package protocol provides public API aliases to internal protocol types.
// This package enables external consumers to use protocol types without
// depending on internal packages.
//
// For future migrations: add new type aliases here as types are promoted
// to public API.
package protocol

import internalprotocol "github.com/tingly-dev/tingly-box/internal/protocol"

// Type aliases to internal protocol types
type APIStyle = internalprotocol.APIStyle
type APIType = internalprotocol.APIType
type Client = internalprotocol.Client
type TokenUsage = internalprotocol.TokenUsage
type OpenAIConfig = internalprotocol.OpenAIConfig

// Re-export constants
const (
	APIStyleOpenAI    APIStyle = internalprotocol.APIStyleOpenAI
	APIStyleAnthropic APIStyle = internalprotocol.APIStyleAnthropic
	APIStyleGoogle    APIStyle = internalprotocol.APIStyleGoogle

	TypeOpenAIChat      APIType = internalprotocol.TypeOpenAIChat
	TypeOpenAIResponses APIType = internalprotocol.TypeOpenAIResponses
	TypeAnthropicV1     APIType = internalprotocol.TypeAnthropicV1
	TypeAnthropicBeta   APIType = internalprotocol.TypeAnthropicBeta
	TypeGoogle          APIType = internalprotocol.TypeGoogle

	CodexAPIBase = internalprotocol.CodexAPIBase
)

// Re-export functions
var (
	NewTokenUsage          = internalprotocol.NewTokenUsage
	NewTokenUsageWithCache = internalprotocol.NewTokenUsageWithCache
	ZeroTokenUsage         = internalprotocol.ZeroTokenUsage
)
