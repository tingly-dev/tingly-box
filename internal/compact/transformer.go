package compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TransformerWrapper adapts strategies to protocol.Transformer interface.
type TransformerWrapper struct {
	strategy CompressionStrategy
}

// NewRoundOnlyTransformer creates a transformer for round-only strategy.
func NewRoundOnlyTransformer() protocol.Transformer {
	return &TransformerWrapper{
		strategy: NewRoundOnlyStrategy(),
	}
}

// NewRoundFilesTransformer creates a transformer for round-files strategy.
func NewRoundFilesTransformer() protocol.Transformer {
	return &TransformerWrapper{
		strategy: NewRoundWithFilesStrategy(),
	}
}

// HandleV1 handles compacting for Anthropic v1 requests.
func (w *TransformerWrapper) HandleV1(req *anthropic.MessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}
	req.Messages = w.strategy.CompressV1(req.Messages)
	return nil
}

// HandleV1Beta handles compacting for Anthropic v1beta requests.
func (w *TransformerWrapper) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}
	req.Messages = w.strategy.CompressBeta(req.Messages)
	return nil
}
