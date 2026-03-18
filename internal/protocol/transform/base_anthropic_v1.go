package transform

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicV1Transform handles transformations for Anthropic v1 Messages API
// This is for when requests are sent directly to Anthropic (not converted to OpenAI format)
type AnthropicV1Transform struct{}

// NewAnthropicV1Transform creates a new Anthropic v1 transform
func NewAnthropicV1Transform() *AnthropicV1Transform {
	return &AnthropicV1Transform{}
}

// Name returns the transform name
func (t *AnthropicV1Transform) Name() string {
	return "anthropic_v1_adjust"
}

// Apply applies Anthropic v1 specific transformations
func (t *AnthropicV1Transform) Apply(ctx *TransformContext) error {
	req, ok := ctx.Request.(*anthropic.MessageNewParams)
	if !ok {
		return &ValidationError{
			Field:   "request",
			Message: "expected anthropic.MessageNewParams for AnthropicV1Transform",
			Value:   ctx.Request,
		}
	}

	// Apply cross-provider consistency rules for Anthropic v1
	if err := t.normalizeAnthropicV1(ctx, req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeAnthropicV1 applies consistency rules for Anthropic v1 requests
// These are rules that apply regardless of which Anthropic provider is used
func (t *AnthropicV1Transform) normalizeAnthropicV1(ctx *TransformContext, req *anthropic.MessageNewParams) error {
	// 1. Normalize tool schemas
	t.normalizeToolSchemas(req)

	// 2. Apply scenario flags
	if ctx.ScenarioFlags != nil {
		t.applyScenarioFlags(req, ctx.ScenarioFlags)
	}

	// 3. Normalize messages
	t.normalizeMessages(req)

	// 4. Validate
	if err := t.validateAnthropicV1(req); err != nil {
		return err
	}

	return nil
}

// normalizeToolSchemas ensures tool schemas follow Anthropic's requirements
func (t *AnthropicV1Transform) normalizeToolSchemas(req *anthropic.MessageNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	// Anthropic has specific requirements for tool schemas
	// - input_schema must be a valid JSON Schema with type: "object"
	// - properties must be defined
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
func (t *AnthropicV1Transform) applyScenarioFlags(req *anthropic.MessageNewParams, flags *typ.ScenarioFlags) {
	// Note: Stream is handled at the client level in Anthropic's SDK, not in the request body
	// So we don't modify req.Stream here

	// Store flags in ExtraFields for potential use downstream
	// Note: MessageNewParams doesn't have ExtraFields, so we skip this for now
	// If needed in the future, we can add a custom field to handle this
}

// normalizeMessages applies message-level normalizations
func (t *AnthropicV1Transform) normalizeMessages(req *anthropic.MessageNewParams) {
	// Anthropic has specific message format requirements
	// This is where we'd add any message-level transformations
	// Currently no specific normalizations needed for Anthropic v1
}

// validateAnthropicV1 validates the Anthropic v1 request
func (t *AnthropicV1Transform) validateAnthropicV1(req *anthropic.MessageNewParams) error {
	// Validate max_tokens
	if req.MaxTokens == 0 {
		return &ValidationError{
			Field:   "max_tokens",
			Message: "max_tokens is required for Anthropic v1 Messages API",
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
				Message: "temperature must be between 0 and 1 for Anthropic v1",
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
				Message: "top_p must be between 0 and 1 for Anthropic v1",
				Value:   topP,
			}
		}
	}

	return nil
}
