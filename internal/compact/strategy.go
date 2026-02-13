// Package compact provides conversation compression strategies.
//
// Strategies compress conversation rounds by removing thinking blocks,
// tool calls, and tool results while preserving the essential flow
// of user requests and assistant responses.
package compact

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// CompressionStrategy defines the interface for compression algorithms.
type CompressionStrategy interface {
	// Name returns the strategy identifier
	Name() string

	// CompressV1 compresses v1 messages
	CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam

	// CompressBeta compresses beta messages
	CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam
}
