package server

import (
	"encoding/json"
)

// =============================================
// Responses API Custom Types
// =============================================
// These are custom types for our internal use.
// Most types are imported from github.com/openai/openai-go/v3/responses

// ResponseInput represents the input for a response (can be string or array of items)
// We need this wrapper because the OpenAI SDK uses a union type
type ResponseInput struct {
	value any
}

// UnmarshalJSON implements custom JSON unmarshaling for ResponseInput
func (r *ResponseInput) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		r.value = str
		return nil
	}
	// Try to unmarshal as array
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		r.value = arr
		return nil
	}
	// Try to unmarshal as single object
	var obj json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		r.value = []json.RawMessage{obj}
		return nil
	}
	return nil
}

// MarshalJSON implements custom JSON marshaling for ResponseInput
func (r *ResponseInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.value)
}

// GetValue returns the underlying value
func (r *ResponseInput) GetValue() any {
	return r.value
}

// IsString checks if the input is a string
func (r *ResponseInput) IsString() bool {
	_, ok := r.value.(string)
	return ok
}

// String returns the string value if applicable
func (r *ResponseInput) String() (string, bool) {
	if s, ok := r.value.(string); ok {
		return s, true
	}
	return "", false
}

// IsArray checks if the input is an array
func (r *ResponseInput) IsArray() bool {
	_, ok := r.value.([]json.RawMessage)
	return ok
}

// =============================================
// Request Wrapper
// =============================================

// ResponseCreateRequest wraps the OpenAI SDK's ResponseNewParams
// We use this for custom JSON unmarshaling if needed
type ResponseCreateRequest struct {
	Model   string         `json:"model" binding:"required"`
	Input   ResponseInput  `json:"input" binding:"required"`
	Stream  bool           `json:"stream,omitempty"`
	// Additional fields can be added as raw JSON to pass through
	Extras map[string]any `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling
func (r *ResponseCreateRequest) UnmarshalJSON(data []byte) error {
	// Use a temporary map to extract known fields and keep extras
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract known fields
	if model, ok := raw["model"].(string); ok {
		r.Model = model
	}
	if stream, ok := raw["stream"].(bool); ok {
		r.Stream = stream
	}

	// Handle input separately
	if inputData, ok := raw["input"]; ok {
		inputJSON, _ := json.Marshal(inputData)
		if err := json.Unmarshal(inputJSON, &r.Input); err != nil {
			return err
		}
	}

	// Store extras (instructions, temperature, tools, etc.)
	delete(raw, "model")
	delete(raw, "input")
	delete(raw, "stream")
	if len(raw) > 0 {
		r.Extras = raw
	}

	return nil
}
