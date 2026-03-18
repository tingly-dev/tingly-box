package transform

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicBetaTransform handles transformations for Anthropic v1beta Messages API
// This is for when requests are sent directly to Anthropic beta endpoints (not converted to OpenAI format)
type AnthropicBetaTransform struct{}

// NewAnthropicBetaTransform creates a new Anthropic beta transform
func NewAnthropicBetaTransform() *AnthropicBetaTransform {
	return &AnthropicBetaTransform{}
}

// Name returns the transform name
func (t *AnthropicBetaTransform) Name() string {
	return "anthropic_beta_adjust"
}

// Apply applies Anthropic beta specific transformations
func (t *AnthropicBetaTransform) Apply(ctx *TransformContext) error {
	req, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	if !ok {
		return &ValidationError{
			Field:   "request",
			Message: "expected anthropic.BetaMessageNewParams for AnthropicBetaTransform",
			Value:   ctx.Request,
		}
	}

	// Apply cross-provider consistency rules for Anthropic beta
	if err := t.normalizeAnthropicBeta(ctx, req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeAnthropicBeta applies consistency rules for Anthropic beta requests
// These are rules that apply regardless of which Anthropic provider is used
func (t *AnthropicBetaTransform) normalizeAnthropicBeta(ctx *TransformContext, req *anthropic.BetaMessageNewParams) error {
	// 1. Normalize tool schemas
	t.normalizeToolSchemas(req)

	// 2. Apply scenario flags
	if ctx.ScenarioFlags != nil {
		t.applyScenarioFlags(req, ctx.ScenarioFlags)
	}

	// 3. Normalize messages
	t.normalizeMessages(req)

	// 4. Validate
	if err := t.validateAnthropicBeta(req); err != nil {
		return err
	}

	return nil
}

// normalizeToolSchemas ensures tool schemas follow Anthropic beta's requirements
func (t *AnthropicBetaTransform) normalizeToolSchemas(req *anthropic.BetaMessageNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	// Anthropic beta may have extended tool schema requirements
	// For now, apply the same basic normalization as v1
	for i := range req.Tools {
		toolUnion := req.Tools[i]
		// Check if this is a regular ToolParam
		if tool := toolUnion.OfTool; tool != nil {
			schema := tool.InputSchema

			// Normalize properties - check if it's a map
			if props, ok := schema.Properties.(map[string]interface{}); ok && len(props) == 0 {
				// If no properties, we should keep the properties field as empty map
				// but normalize the schema
			}
		}
	}
}

// applyScenarioFlags applies scenario-specific flags to the request
func (t *AnthropicBetaTransform) applyScenarioFlags(req *anthropic.BetaMessageNewParams, flags *typ.ScenarioFlags) {
	// Note: Stream is handled at the client level in Anthropic's SDK, not in the request body
	// So we don't modify req.Stream here

	// Store flags in ExtraFields for potential use downstream
	// Note: BetaMessageNewParams doesn't have ExtraFields, so we skip this for now
	// If needed in the future, we can add a custom field to handle this
}

// normalizeMessages applies message-level normalizations
func (t *AnthropicBetaTransform) normalizeMessages(req *anthropic.BetaMessageNewParams) {
	// Anthropic beta supports additional message types
	// This is where we'd add any message-level transformations
	// Currently no specific normalizations needed for Anthropic beta
}

// validateAnthropicBeta validates the Anthropic beta request
func (t *AnthropicBetaTransform) validateAnthropicBeta(req *anthropic.BetaMessageNewParams) error {
	// Validate max_tokens
	if req.MaxTokens == 0 {
		return &ValidationError{
			Field:   "max_tokens",
			Message: "max_tokens is required for Anthropic beta Messages API",
			Value:   req.MaxTokens,
		}
	}

	// Validate model
	if req.Model == "" {
		return &ValidationError{
			Field:   "model",
			Message: "model is required",
			Value:   req.Model,
		}
	}

	// Validate temperature range (Anthropic: 0-1)
	if req.Temperature.Valid() {
		temp := req.Temperature.Value
		if temp < 0 || temp > 1 {
			return &ValidationError{
				Field:   "temperature",
				Message: "temperature must be between 0 and 1 for Anthropic beta",
				Value:   temp,
			}
		}
	}

	// Validate top_p range (Anthropic: 0-1)
	if req.TopP.Valid() {
		topP := req.TopP.Value
		if topP < 0 || topP > 1 {
			return &ValidationError{
				Field:   "top_p",
				Message: "top_p must be between 0 and 1 for Anthropic beta",
				Value:   topP,
			}
		}
	}

	return nil
}
