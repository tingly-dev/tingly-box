package transform

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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
	targetAPIStyle protocol.APIType
}

// NewConsistencyTransform creates a new ConsistencyTransform for the given target API style.
func NewConsistencyTransform(targetAPIStyle protocol.APIType) *ConsistencyTransform {
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
	case protocol.TypeOpenAIChat:
		return t.normalizeChatCompletion(ctx)
	case protocol.TypeOpenAIResponses:
		return t.normalizeResponses(ctx)
	case protocol.TypeAnthropicV1:
		return t.normalizeAnthropicV1(ctx)
	case protocol.TypeAnthropicBeta:
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
		return &ValidationError{
			Field:   "request",
			Message: fmt.Sprintf("expected *openai.ChatCompletionNewParams for OpenAI Chat normalization, got %T", ctx.Request),
			Value:   ctx.Request,
		}
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

	// First, align tool messages to ensure they follow assistant messages with tool_calls
	t.alignToolMessages(req)

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

// AlignToolMessagesForOpenAI converts orphaned tool messages (those without a
// matching tool_call_id) to user messages. This prevents "role 'tool' must be a
// response to preceding message with 'tool_calls'" errors.
func AlignToolMessagesForOpenAI(req *openai.ChatCompletionNewParams) {
	if len(req.Messages) == 0 {
		return
	}

	// Collect all valid tool_call_ids from assistant messages
	validToolCallIDs := make(map[string]bool)
	for _, msg := range req.Messages {
		if msg.OfAssistant == nil {
			continue
		}
		for _, tc := range msg.OfAssistant.ToolCalls {
			if id := tc.GetID(); id != nil && *id != "" {
				validToolCallIDs[*id] = true
			}
		}
	}

	// Convert orphaned tool messages to user messages
	for i := range req.Messages {
		if req.Messages[i].OfTool != nil {
			toolMsg := req.Messages[i].OfTool
			toolCallID := toolMsg.ToolCallID
			if toolCallID == "" || validToolCallIDs[toolCallID] {
				continue
			}

			// Orphaned tool message, convert to user message.
			// Tool content supports string or text parts; map both to user content.
			if toolMsg.Content.OfString.Valid() {
				req.Messages[i] = openai.UserMessage(toolMsg.Content.OfString.Value)
				continue
			}

			if len(toolMsg.Content.OfArrayOfContentParts) > 0 {
				parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(toolMsg.Content.OfArrayOfContentParts))
				for _, part := range toolMsg.Content.OfArrayOfContentParts {
					parts = append(parts, openai.TextContentPart(part.Text))
				}
				req.Messages[i] = openai.UserMessage(parts)
				continue
			}

			req.Messages[i] = openai.UserMessage("")
		}
	}
}

// alignToolMessages ensures tool messages follow assistant messages with tool_calls.
// OpenAI API requires that messages with role "tool" must be a response to a preceding
// message with "tool_calls". This function converts orphaned tool messages to user messages.
func (t *ConsistencyTransform) alignToolMessages(req *openai.ChatCompletionNewParams) {
	// Delegate to the public function
	AlignToolMessagesForOpenAI(req)
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
		return &ValidationError{
			Field:   "request",
			Message: fmt.Sprintf("expected *responses.ResponseNewParams for Responses API normalization, got %T", ctx.Request),
			Value:   ctx.Request,
		}
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
func (t *ConsistencyTransform) normalizeAnthropicV1(ctx *TransformContext) error {
	req, ok := ctx.Request.(*anthropic.MessageNewParams)
	if !ok {
		return &ValidationError{
			Field:   "request",
			Message: fmt.Sprintf("expected *anthropic.MessageNewParams for Anthropic v1 normalization, got %T", ctx.Request),
			Value:   ctx.Request,
		}
	}

	// 1. Normalize tool schemas
	t.normalizeAnthropicV1ToolSchemas(req)

	// 2. Apply scenario flags
	if ctx.ScenarioFlags != nil {
		t.applyAnthropicV1ScenarioFlags(req, ctx.ScenarioFlags)
	}

	// 3. Normalize messages
	t.normalizeAnthropicV1Messages(req)

	// 4. Validate
	if err := t.validateAnthropicV1(req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeAnthropicBeta applies consistency rules to Anthropic beta requests.
func (t *ConsistencyTransform) normalizeAnthropicBeta(ctx *TransformContext) error {
	req, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	if !ok {
		return &ValidationError{
			Field:   "request",
			Message: fmt.Sprintf("expected *anthropic.BetaMessageNewParams for Anthropic beta normalization, got %T", ctx.Request),
			Value:   ctx.Request,
		}
	}

	// 1. Normalize tool schemas
	t.normalizeAnthropicBetaToolSchemas(req)

	// 2. Apply scenario flags
	if ctx.ScenarioFlags != nil {
		t.applyAnthropicBetaScenarioFlags(req, ctx.ScenarioFlags)
	}

	// 3. Normalize messages
	t.normalizeAnthropicBetaMessages(req)

	// 4. Validate
	if err := t.validateAnthropicBeta(req); err != nil {
		return err
	}

	ctx.Request = req
	return nil
}

// normalizeAnthropicV1ToolSchemas ensures tool schemas follow Anthropic's requirements
func (t *ConsistencyTransform) normalizeAnthropicV1ToolSchemas(req *anthropic.MessageNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	// Anthropic has specific requirements for tool schemas
	// - input_schema must be a valid JSON Schema with type: "object"
	// - properties must be defined
	for i := range req.Tools {
		toolUnion := req.Tools[i]
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

// applyAnthropicV1ScenarioFlags applies scenario-specific flags to the request
func (t *ConsistencyTransform) applyAnthropicV1ScenarioFlags(req *anthropic.MessageNewParams, flags *typ.ScenarioFlags) {
	// Note: Stream is handled at the client level in Anthropic's SDK, not in the request body
	// So we don't modify req.Stream here

	// Store flags in ExtraFields for potential use downstream
	// Note: MessageNewParams doesn't have ExtraFields, so we skip this for now
	// If needed in the future, we can add a custom field to handle this
}

// normalizeAnthropicV1Messages applies message-level normalizations
func (t *ConsistencyTransform) normalizeAnthropicV1Messages(req *anthropic.MessageNewParams) {
	// Anthropic has specific message format requirements
	// This is where we'd add any message-level transformations
	// Currently no specific normalizations needed for Anthropic v1
}

// validateAnthropicV1 validates the Anthropic v1 request
func (t *ConsistencyTransform) validateAnthropicV1(req *anthropic.MessageNewParams) error {
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

// normalizeAnthropicBetaToolSchemas ensures tool schemas follow Anthropic beta's requirements
func (t *ConsistencyTransform) normalizeAnthropicBetaToolSchemas(req *anthropic.BetaMessageNewParams) {
	if len(req.Tools) == 0 {
		return
	}

	// Anthropic beta may have extended tool schema requirements
	// For now, apply the same basic normalization as v1
	for i := range req.Tools {
		toolUnion := req.Tools[i]
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

// applyAnthropicBetaScenarioFlags applies scenario-specific flags to the request
func (t *ConsistencyTransform) applyAnthropicBetaScenarioFlags(req *anthropic.BetaMessageNewParams, flags *typ.ScenarioFlags) {
	// Note: Stream is handled at the client level in Anthropic's SDK, not in the request body
	// So we don't modify req.Stream here

	// Store flags in ExtraFields for potential use downstream
	// Note: BetaMessageNewParams doesn't have ExtraFields, so we skip this for now
	// If needed in the future, we can add a custom field to handle this
}

// normalizeAnthropicBetaMessages applies message-level normalizations
func (t *ConsistencyTransform) normalizeAnthropicBetaMessages(req *anthropic.BetaMessageNewParams) {
	// Anthropic beta supports additional message types
	// This is where we'd add any message-level transformations
	// Currently no specific normalizations needed for Anthropic beta
}

// validateAnthropicBeta validates the Anthropic beta request
func (t *ConsistencyTransform) validateAnthropicBeta(req *anthropic.BetaMessageNewParams) error {
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
