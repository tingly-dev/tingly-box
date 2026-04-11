package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// TransformerToTransformAdapter bridges the old protocol.Transformer interface
// to the new transform.Transform interface. This allows smooth migration
// of the codebase to the new transform architecture.
type TransformerToTransformAdapter struct {
	transformer protocol.Transformer
}

// NewTransformerToTransformAdapter creates a new adapter from a protocol.Transformer.
func NewTransformerToTransformAdapter(t protocol.Transformer) transform.Transform {
	return &TransformerToTransformAdapter{
		transformer: t,
	}
}

// Name returns the transform identifier.
func (a *TransformerToTransformAdapter) Name() string {
	// Try to get name from the underlying transformer
	if named, ok := a.transformer.(interface{ Name() string }); ok {
		return named.Name()
	}
	return "transformer_adapter"
}

// Apply applies the transformation by dispatching to the appropriate Handle method.
func (a *TransformerToTransformAdapter) Apply(ctx *transform.TransformContext) error {
	switch r := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return a.transformer.HandleV1(r)
	case *anthropic.BetaMessageNewParams:
		return a.transformer.HandleV1Beta(r)

	default:
		// Unsupported request type, pass through
		return nil
	}
}
