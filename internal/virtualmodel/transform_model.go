package virtualmodel

import (
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// TransformModelConfig holds configuration for a transform virtual model (proxy).
type TransformModelConfig struct {
	ID          string
	Name        string
	Description string
	Transformer protocol.Transformer      // applied after Chain
	Chain       *transform.TransformChain // applied first
}

// TransformModel implements AnthropicVirtualModel for proxy/transform virtual models.
// HandleAnthropic applies Chain and Transformer in-place on req; ResponseAnthropic reads the result.
// It does NOT implement OpenAIChatVirtualModel — transform models are Anthropic-only.
type TransformModel struct {
	cfg *TransformModelConfig
}

// Compile-time interface check.
var _ AnthropicVirtualModel = (*TransformModel)(nil)

// NewTransformModel creates a TransformModel.
func NewTransformModel(cfg *TransformModelConfig) *TransformModel {
	return &TransformModel{cfg: cfg}
}

func (m *TransformModel) GetID() string { return m.cfg.ID }

// SimulatedDelay is always 0 — transform models don't simulate latency.
func (m *TransformModel) SimulatedDelay() time.Duration { return 0 }

// Protocols declares that TransformModel is Anthropic-only.
func (m *TransformModel) Protocols() []protocol.APIType {
	return []protocol.APIType{protocol.TypeAnthropicBeta}
}

func (m *TransformModel) ToModel() Model {
	return Model{
		ID:      m.cfg.ID,
		Object:  "model",
		Created: 0,
		OwnedBy: "tingly-box-virtual",
	}
}

// HandleAnthropicStream delegates to DefaultAnthropicStream since TransformModel is batch-only.
func (m *TransformModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	return DefaultAnthropicStream(m, req, emit)
}

// HandleAnthropic applies Chain then Transformer to req in-place and returns the response.
func (m *TransformModel) HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	if m.cfg.Chain != nil {
		ctx := transform.NewTransformContext(&req.BetaMessageNewParams)
		if _, err := m.cfg.Chain.Execute(ctx); err != nil {
			return VModelResponse{}, fmt.Errorf("transform chain failed: %w", err)
		}
	}
	if m.cfg.Transformer != nil {
		if err := m.cfg.Transformer.HandleV1Beta(&req.BetaMessageNewParams); err != nil {
			return VModelResponse{}, fmt.Errorf("transformer failed: %w", err)
		}
	}

	text := ""
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		for _, blk := range last.Content {
			if blk.OfText != nil {
				text += blk.OfText.Text
			}
		}
	}
	return VModelResponse{
		Content:    []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: text}}},
		StopReason: "end_turn",
	}, nil
}
