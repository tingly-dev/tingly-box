package protocol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// =============================================
//Chat Completion API
// =============================================

// OpenAIChatCompletionRequest is a type alias for OpenAI chat completion request with extra fields.
type OpenAIChatCompletionRequest struct {
	*openai.ChatCompletionNewParams
	Stream bool `json:"stream"`
}

func (r *OpenAIChatCompletionRequest) MarshalJSON() ([]byte, error) {
	inner, err := json.Marshal(r.ChatCompletionNewParams)
	if err != nil {
		return nil, err
	}
	if !r.Stream {
		return inner, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(inner, &raw); err != nil {
		return nil, err
	}
	raw["stream"] = json.RawMessage("true")
	return json.Marshal(raw)
}

func (r *OpenAIChatCompletionRequest) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	r.ChatCompletionNewParams = new(openai.ChatCompletionNewParams)
	r.Stream = aux.Stream

	if err := json.Unmarshal(data, r.ChatCompletionNewParams); err != nil {
		return err
	}
	return nil
}

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
	*responses.ResponseNewParams
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

	// Preprocess the JSON to add "type": "message" to input items that don't have it
	// This is needed because the OpenAI SDK's union deserializer requires the type field
	processedData, err := PreprocessInputData(data)
	if err != nil {
		return err
	}

	// Then, unmarshal into the embedded ResponseNewParams
	r.ResponseNewParams = new(responses.ResponseNewParams)
	r.Stream = aux.Stream

	if err := json.Unmarshal(processedData, r.ResponseNewParams); err != nil {
		return err
	}
	return nil
}

// PreprocessInputData preprocesses the JSON data before unmarshaling.
// It performs two preprocessing steps:
// 1. Adds "type": "message" to input items that don't have a type field
// 2. Flattens output_text content blocks into single strings
//
// Items are inspected with gjson and only the ones that actually need a
// rewrite are re-serialized; untouched requests are returned as-is instead of
// being decoded and re-encoded item by item.
func PreprocessInputData(data []byte) ([]byte, error) {
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("invalid JSON in request body")
	}
	input := gjson.GetBytes(data, "input")
	if !input.Exists() || !input.IsArray() {
		// No input, or input is not an array (might be a string): return as-is.
		return data, nil
	}

	items := input.Array()
	var modified map[int]string

	for i, item := range items {
		if !item.IsObject() {
			continue
		}

		// Step 1: Infer missing union discriminator for input items.
		// Some clients replay prior Responses history without `type`, but the SDK
		// needs it to deserialize union items and preserve function arguments.
		itemType := item.Get("type").Str
		inferredType := ""
		if !item.Get("type").Exists() {
			switch {
			case item.Get("role").Exists():
				inferredType = "message"
			case item.Get("call_id").Exists() && item.Get("arguments").Exists():
				inferredType = "function_call"
			case item.Get("call_id").Exists() && item.Get("output").Exists():
				inferredType = "function_call_output"
			}
			if inferredType != "" {
				itemType = inferredType
			}
		}

		// Step 2: Flatten output_text content blocks
		flattened, needsFlatten := "", false
		if itemType == "message" {
			if content := item.Get("content"); content.IsArray() {
				flattened, needsFlatten = flattenOutputTextContent(content)
			}
		}

		if inferredType == "" && !needsFlatten {
			continue
		}

		itemRaw := item.Raw
		var err error
		if inferredType != "" {
			if itemRaw, err = sjson.Set(itemRaw, "type", inferredType); err != nil {
				continue
			}
		}
		if needsFlatten {
			if itemRaw, err = sjson.Set(itemRaw, "content", flattened); err != nil {
				continue
			}
		}
		if modified == nil {
			modified = make(map[int]string)
		}
		modified[i] = itemRaw
	}

	if len(modified) == 0 {
		return data, nil
	}

	// Rebuild the input array once, splicing in the rewritten items.
	var buf strings.Builder
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		if m, ok := modified[i]; ok {
			buf.WriteString(m)
		} else {
			buf.WriteString(item.Raw)
		}
	}
	buf.WriteByte(']')

	out, err := sjson.SetRawBytes(data, "input", []byte(buf.String()))
	if err != nil {
		return data, nil
	}
	return out, nil
}

// flattenOutputTextContent flattens output_text blocks into a single string.
//
// The Responses API returns output_text in responses, but when using those
// responses as history in subsequent requests, we need to flatten them to
// strings for SDK compatibility.
func flattenOutputTextContent(content gjson.Result) (string, bool) {
	var parts []string
	for _, item := range content.Array() {
		if !item.IsObject() {
			continue
		}
		if item.Get("type").Str != "output_text" {
			continue
		}
		if text := item.Get("text").Str; text != "" {
			parts = append(parts, text)
		}
	}

	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
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
