package anthropic

import (
	"fmt"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// TransformModelConfig holds configuration for a transform virtual model (proxy).
type TransformModelConfig struct {
	ID          string
	Name        string
	Description string
	Transformer transform.Transform       // applied after Chain
	Chain       *transform.TransformChain // applied first
}

// TransformModel is an Anthropic-protocol proxy model. HandleAnthropic
// applies Chain and Transformer in-place on req and returns the resulting
// last-message text.
type TransformModel struct {
	cfg *TransformModelConfig
}

// Compile-time interface check.
var _ VirtualModel = (*TransformModel)(nil)

// NewTransformModel creates a TransformModel.
func NewTransformModel(cfg *TransformModelConfig) *TransformModel {
	return &TransformModel{cfg: cfg}
}

func (m *TransformModel) GetID() string { return m.cfg.ID }

func (m *TransformModel) GetName() string { return m.cfg.Name }

func (m *TransformModel) GetDescription() string { return m.cfg.Description }

func (m *TransformModel) GetType() virtualmodel.VirtualModelType {
	return virtualmodel.VirtualModelTypeProxy
}

// SimulatedDelay is always 0 — transform models don't simulate latency.
func (m *TransformModel) SimulatedDelay() time.Duration { return 0 }

func (m *TransformModel) ToModel() virtualmodel.Model {
	return virtualmodel.Model{
		ID:      m.cfg.ID,
		Object:  "model",
		Created: 0,
		OwnedBy: "tingly-box-virtual",
	}
}

// HandleAnthropicStream delegates to DefaultStream since TransformModel is batch-only.
func (m *TransformModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	return DefaultStream(m, req, emit)
}

// HandleAnthropic applies Chain then Transformer to req in-place and returns the response.
func (m *TransformModel) HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	ctx := transform.NewTransformContext(&req.BetaMessageNewParams)

	if m.cfg.Chain != nil {
		if _, err := m.cfg.Chain.Execute(ctx); err != nil {
			return VModelResponse{}, fmt.Errorf("transform chain failed: %w", err)
		}
	}

	if m.cfg.Transformer != nil {
		if err := m.cfg.Transformer.Apply(ctx); err != nil {
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
		Content:    []sdk.BetaContentBlockParamUnion{{OfText: &sdk.BetaTextBlockParam{Text: text}}},
		StopReason: "end_turn",
	}, nil
}
