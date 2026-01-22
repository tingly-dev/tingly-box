package transform

import "github.com/anthropics/anthropic-sdk-go"

// Transformer defines the interface for request compacting transformations.
// Each handler method is responsible for a different request model type.
type Transformer interface {
	// HandleV1 handles compacting for Anthropic v1 requests.
	HandleV1(req *anthropic.MessageNewParams) error

	// HandleV1Beta handles compacting for Anthropic v1beta requests.
	HandleV1Beta(req *anthropic.BetaMessageNewParams) error
}
