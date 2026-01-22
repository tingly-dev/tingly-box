package request

import (
	"encoding/json"

	"github.com/openai/openai-go/v3"
)

// OpenAIChatCompletionRequest is a type alias for OpenAI chat completion request with extra fields.
type OpenAIChatCompletionRequest struct {
	openai.ChatCompletionNewParams
	Stream bool `json:"stream"`
}

func (r *OpenAIChatCompletionRequest) UnmarshalJSON(data []byte) error {
	var inner openai.ChatCompletionNewParams
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}
	r.Stream = aux.Stream
	r.ChatCompletionNewParams = inner
	return nil
}
