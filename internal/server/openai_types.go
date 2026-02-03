package server

import (
	"encoding/json"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// =============================================
// Responses API Custom Types
// =============================================
// These types wrap the native OpenAI SDK types to add
// additional fields that are needed for our proxy but not
// part of the native SDK types.
//
// Following the same pattern as anthropic.go

// ResponseCreateRequest wraps the native ResponseNewParams with additional fields
// for proxy-specific handling like the `stream` parameter.
type ResponseCreateRequest struct {
	// Stream indicates whether to stream the response
	// This is not part of ResponseNewParams as streaming is controlled
	// by using NewStreaming() method on the SDK client
	Stream bool `json:"stream"`

	// Embed the native SDK type for all other fields
	responses.ResponseNewParams
}

// UnmarshalJSON implements custom JSON unmarshaling for ResponseCreateRequest
// It handles both the custom Stream field and the embedded ResponseNewParams
func (r *ResponseCreateRequest) UnmarshalJSON(data []byte) error {
	// First, extract the Stream field
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Then, unmarshal into the embedded ResponseNewParams
	var inner responses.ResponseNewParams
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}

	r.Stream = aux.Stream
	r.ResponseNewParams = inner
	return nil
}

// =============================================
// Type Aliases for Native SDK Types
// =============================================
// These aliases provide convenient access to the native OpenAI SDK types

// ResponseNewParams is an alias to the native OpenAI SDK type
type ResponseNewParams = responses.ResponseNewParams

// Response is an alias to the native OpenAI SDK type
type Response = responses.Response

// ResponseInputItemUnionParam is an alias to the native OpenAI SDK type
type ResponseInputItemUnionParam = responses.ResponseInputItemUnionParam

// ResponseNewParamsInputUnion is an alias to the native OpenAI SDK type
type ResponseNewParamsInputUnion = responses.ResponseNewParamsInputUnion

// =============================================
// Helper Functions for Native Types
// =============================================

// GetInputValue extracts the raw input value from ResponseNewParamsInputUnion.
// Returns the underlying string, array, or nil.
func GetInputValue(input responses.ResponseNewParamsInputUnion) any {
	if !param.IsOmitted(input.OfString) {
		return input.OfString.Value
	} else if !param.IsOmitted(input.OfInputItemList) {
		return input.OfInputItemList
	}
	return nil
}
