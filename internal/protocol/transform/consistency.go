package transform

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConsistencyTransform applies cross-provider normalization rules to requests.
// These rules apply to ALL providers, regardless of vendor.
//
// Consistency Transform handles:
//   - Tool Schema Normalization - Ensure type: "object", normalize properties
//   - Scenario Flags - Disable stream usage, thinking mode if needed
//   - Messages Normalization - Truncate tool_call_id to 40 chars
//   - Validation - Check max_tokens, temperature ranges
type ConsistencyTransform struct {
	targetAPIStyle TargetAPIStyle
}

// NewConsistencyTransform creates a new ConsistencyTransform for the given target API style.
func NewConsistencyTransform(targetAPIStyle TargetAPIStyle) *ConsistencyTransform {
	return &ConsistencyTransform{
		targetAPIStyle: targetAPIStyle,
	}
}

// Name returns the transform name for logging and tracking.
func (t *ConsistencyTransform) Name() string {
	return "consistency_normalize"
}

// Apply executes the consistency normalization based on the target API style.
// Modifies ctx.Request in place and returns an error if transformation fails.
func (t *ConsistencyTransform) Apply(ctx *TransformContext) error {
	switch t.targetAPIStyle {
	case TargetAPIStyleOpenAIChat:
		return t.normalizeChatCompletion(ctx)
	case TargetAPIStyleOpenAIResponses:
		return t.normalizeResponses(ctx)
	case TargetAPIStyleAnthropicV1:
		return t.normalizeAnthropicV1(ctx)
	case TargetAPIStyleAnthropicBeta:
		return t.normalizeAnthropicBeta(ctx)
	default:
		// No transformation for unknown API styles
		return nil
	}
}

// normalizeChatCompletion applies consistency rules to OpenAI Chat Completions requests.
func (t *ConsistencyTransform) normalizeChatCompletion(ctx *TransformContext) error {
	req, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		return nil
	}

	// 1. Normalize tool schemas (all providers)
	t.normalizeToolSchemas(req)

	// 2. Apply scenario flags (all providers)
	if ctx.ScenarioFlags != nil {
		t.applyScenarioFlags(req, ctx.ScenarioFlags, ctx.IsStreaming)
	}

	// 3. Normalize messages (e.g., tool_call_id truncation - all providers)
	t.normalizeMessages(req)

	// 4. Validate (all providers)
	if err := t.validateChatCompletion(req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeToolSchemas ensures tool schemas have proper structure.
// This applies to ALL providers.
//
// Normalization rules:
//   - Ensure parameters type is "object"
//   - Ensure properties exist if parameters are present
//   - Normalize empty parameters to nil (avoids sending empty objects)
func (t *ConsistencyTransform) normalizeToolSchemas(req *openai.ChatCompletionNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	for i, toolUnion := range req.Tools {
		if toolUnion.OfFunction == nil {
			continue
		}

		fn := toolUnion.OfFunction.Function

		// Normalize tool parameters schema
		if len(fn.Parameters) > 0 {
			// Ensure type is "object" if not specified
			if _, hasType := fn.Parameters["type"]; !hasType {
				fn.Parameters["type"] = "object"
			}

			// If type is "object" but properties is missing/empty, normalize
			if fn.Parameters["type"] == "object" {
				if props, hasProps := fn.Parameters["properties"]; !hasProps || props == nil {
					// Add empty properties to ensure valid schema
					fn.Parameters["properties"] = map[string]interface{}{}
				}
			}

			// Normalize: remove empty parameters map to avoid sending empty objects
			if len(fn.Parameters) == 1 && fn.Parameters["type"] == "object" {
				props, hasProps := fn.Parameters["properties"]
				if !hasProps || (len(props.(map[string]interface{})) == 0) {
					// Empty parameters, set to nil to avoid sending empty object
					req.Tools[i].OfFunction.Function.Parameters = nil
				}
			}
		}
	}
}

// applyScenarioFlags applies scenario-specific configuration flags.
// This applies to ALL providers.
//
// Scenario flags handled:
//   - DisableStreamUsage: Don't include usage in streaming chunks (for incompatible clients)
//   - ThinkingEffort/ThinkingMode: Override thinking mode (via OpenAIConfig in ExtraFields)
func (t *ConsistencyTransform) applyScenarioFlags(req *openai.ChatCompletionNewParams, flags *typ.ScenarioFlags, isStreaming bool) {
	// Handle stream_options - disable usage in streaming if requested
	if isStreaming && flags.DisableStreamUsage {
		if req.StreamOptions.IncludeUsage.Value {
			req.StreamOptions.IncludeUsage.Value = false
		}
	}

	// Store scenario flags in ExtraFields for downstream use
	extraFields := req.ExtraFields()
	if extraFields == nil {
		extraFields = map[string]any{}
	}
	extraFields["scenario_flags"] = flags
	req.SetExtraFields(extraFields)
}

// normalizeMessages normalizes message fields for cross-provider compatibility.
// This applies to ALL providers.
//
// Normalization rules:
//   - Truncate tool_call_id to 40 characters (OpenAI API requirement)
func (t *ConsistencyTransform) normalizeMessages(req *openai.ChatCompletionNewParams) {
	if len(req.Messages) == 0 {
		return
	}

	for i := range req.Messages {
		// Check if this is a tool message
		if req.Messages[i].OfTool != nil {
			// Convert to map to access tool_call_id
			msgMap := req.Messages[i].ExtraFields()
			if msgMap == nil {
				// Try to unmarshal the message to get tool_call_id
				if msgBytes, err := json.Marshal(req.Messages[i]); err == nil {
					var toolMsg map[string]interface{}
					if err := json.Unmarshal(msgBytes, &toolMsg); err == nil {
						if toolCallID, ok := toolMsg["tool_call_id"].(string); ok {
							// Truncate tool_call_id if needed
							if len(toolCallID) > maxToolCallIDLength {
								truncatedID := toolCallID[:maxToolCallIDLength-3] + "..."
								toolMsg["tool_call_id"] = truncatedID

								// Re-marshal and unmarshal to update message
								if newBytes, err := json.Marshal(toolMsg); err == nil {
									var updatedMsg openai.ChatCompletionMessageParamUnion
									if err := json.Unmarshal(newBytes, &updatedMsg); err == nil {
										req.Messages[i] = updatedMsg
									}
								}
							}
						}
					}
				}
			} else {
				// tool_call_id should be in the message structure, not ExtraFields
				// For OpenAI ChatCompletionMessageParamUnion.OfTool, tool_call_id is a direct field
				// We need to handle this differently - let's check the actual structure

				// Re-marshal and inspect the message
				if msgBytes, err := json.Marshal(req.Messages[i]); err == nil {
					var toolMsg map[string]interface{}
					if err := json.Unmarshal(msgBytes, &toolMsg); err == nil {
						if toolCallID, ok := toolMsg["tool_call_id"].(string); ok {
							if len(toolCallID) > maxToolCallIDLength {
								truncatedID := toolCallID[:maxToolCallIDLength-3] + "..."
								toolMsg["tool_call_id"] = truncatedID

								if newBytes, err := json.Marshal(toolMsg); err == nil {
									var updatedMsg openai.ChatCompletionMessageParamUnion
									if err := json.Unmarshal(newBytes, &updatedMsg); err == nil {
										req.Messages[i] = updatedMsg
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// validateChatCompletion validates request parameters against OpenAI API constraints.
// This applies to ALL providers.
//
// Validation rules:
//   - Temperature: Must be between 0 and 2 (inclusive)
//   - MaxTokens: Should be positive if specified
//   - TopP: Must be between 0 and 1 (inclusive)
func (t *ConsistencyTransform) validateChatCompletion(req *openai.ChatCompletionNewParams) error {
	// Validate temperature: 0 <= temperature <= 2
	if req.Temperature.Value < 0 || req.Temperature.Value > 2 {
		return &ValidationError{
			Field:   "temperature",
			Message: "temperature must be between 0 and 2",
			Value:   req.Temperature.Value,
		}
	}

	// Validate max_tokens: should be positive if specified
	if req.MaxTokens.Value < 0 {
		return &ValidationError{
			Field:   "max_tokens",
			Message: "max_tokens must be non-negative",
			Value:   req.MaxTokens.Value,
		}
	}

	// Validate top_p: 0 <= top_p <= 1
	if req.TopP.Value < 0 || req.TopP.Value > 1 {
		return &ValidationError{
			Field:   "top_p",
			Message: "top_p must be between 0 and 1",
			Value:   req.TopP.Value,
		}
	}

	return nil
}

// normalizeResponses applies consistency rules to OpenAI Responses API requests.
func (t *ConsistencyTransform) normalizeResponses(ctx *TransformContext) error {
	req, ok := ctx.Request.(*responses.ResponseNewParams)
	if !ok {
		return nil
	}

	// 1. Normalize tool schemas (all providers)
	t.normalizeResponseToolSchemas(req)

	// 2. Apply scenario flags (all providers)
	if ctx.ScenarioFlags != nil {
		t.applyResponseScenarioFlags(req, ctx.ScenarioFlags, ctx.IsStreaming)
	}

	// 3. Validate (all providers)
	if err := t.validateResponses(req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeResponseToolSchemas ensures tool schemas have proper structure for Responses API.
// This applies to ALL providers.
//
// Normalization rules:
//   - Ensure parameters type is "object"
//   - Ensure properties exist if parameters are present
//   - Normalize empty parameters to nil (avoids sending empty objects)
func (t *ConsistencyTransform) normalizeResponseToolSchemas(req *responses.ResponseNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	for i, toolUnion := range req.Tools {
		if toolUnion.OfFunction == nil {
			continue
		}

		fn := toolUnion.OfFunction

		// Normalize tool parameters schema
		if len(fn.Parameters) > 0 {
			// Ensure type is "object" if not specified
			if _, hasType := fn.Parameters["type"]; !hasType {
				fn.Parameters["type"] = "object"
			}

			// If type is "object" but properties is missing/empty, normalize
			if fn.Parameters["type"] == "object" {
				if props, hasProps := fn.Parameters["properties"]; !hasProps || props == nil {
					// Add empty properties to ensure valid schema
					fn.Parameters["properties"] = map[string]interface{}{}
				}
			}

			// Normalize: remove empty parameters map to avoid sending empty objects
			if len(fn.Parameters) == 1 && fn.Parameters["type"] == "object" {
				props, hasProps := fn.Parameters["properties"]
				if !hasProps || (len(props.(map[string]interface{})) == 0) {
					// Empty parameters, set to nil to avoid sending empty object
					req.Tools[i].OfFunction.Parameters = nil
				}
			}
		}
	}
}

// applyResponseScenarioFlags applies scenario-specific configuration flags for Responses API.
// This applies to ALL providers.
//
// Scenario flags handled:
//   - DisableStreamUsage: Don't include obfuscation in streaming chunks (for incompatible clients)
func (t *ConsistencyTransform) applyResponseScenarioFlags(req *responses.ResponseNewParams, flags *typ.ScenarioFlags, isStreaming bool) {
	// Handle stream_options - disable obfuscation in streaming if requested
	if isStreaming && flags.DisableStreamUsage {
		if req.StreamOptions.IncludeObfuscation.Value {
			req.StreamOptions.IncludeObfuscation.Value = false
		}
	}

	// Store scenario flags in ExtraFields for downstream use
	extraFields := req.ExtraFields()
	if extraFields == nil {
		extraFields = map[string]any{}
	}
	extraFields["scenario_flags"] = flags
	req.SetExtraFields(extraFields)
}

// validateResponses validates request parameters against OpenAI Responses API constraints.
// This applies to ALL providers.
//
// Validation rules:
//   - Temperature: Must be between 0 and 2 (inclusive)
//   - MaxOutputTokens: Should be positive if specified
//   - TopP: Must be between 0 and 1 (inclusive)
func (t *ConsistencyTransform) validateResponses(req *responses.ResponseNewParams) error {
	// Validate temperature: 0 <= temperature <= 2
	if !req.Temperature.Valid() && (req.Temperature.Value < 0 || req.Temperature.Value > 2) {
		return &ValidationError{
			Field:   "temperature",
			Message: "temperature must be between 0 and 2",
			Value:   req.Temperature.Value,
		}
	}

	// Validate max_output_tokens: should be positive if specified
	if !req.MaxOutputTokens.Valid() && req.MaxOutputTokens.Value < 0 {
		return &ValidationError{
			Field:   "max_output_tokens",
			Message: "max_output_tokens must be non-negative",
			Value:   req.MaxOutputTokens.Value,
		}
	}

	// Validate top_p: 0 <= top_p <= 1
	if !req.TopP.Valid() && (req.TopP.Value < 0 || req.TopP.Value > 1) {
		return &ValidationError{
			Field:   "top_p",
			Message: "top_p must be between 0 and 1",
			Value:   req.TopP.Value,
		}
	}

	return nil
}

// normalizeAnthropicV1 applies consistency rules to Anthropic v1 requests.
// Placeholder for Phase 3 implementation.
func (t *ConsistencyTransform) normalizeAnthropicV1(ctx *TransformContext) error {
	// Phase 3: Implement Anthropic v1 normalization
	return nil
}

// normalizeAnthropicBeta applies consistency rules to Anthropic beta requests.
// Placeholder for Phase 3 implementation.
func (t *ConsistencyTransform) normalizeAnthropicBeta(ctx *TransformContext) error {
	// Phase 3: Implement Anthropic beta normalization
	return nil
}

// Constants
const (
	// maxToolCallIDLength is the maximum length for tool_call_id in OpenAI API
	// OpenAI API requires tool_call.id to be <= 40 characters
	maxToolCallIDLength = 40
)

// ValidationError represents a validation error for request parameters.
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Value != nil {
		return "validation error: " + e.Message + " (field: " + e.Field + ", value: " + fmt.Sprintf("%v", e.Value) + ")"
	}
	return "validation error: " + e.Message + " (field: " + e.Field + ")"
}
