package assembler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// StreamAssembler is the protocol-owned common surface for reconstructing one
// complete response from native stream events. It accepts SDK events, Wire
// DTOs, or json.RawMessage values; protocol-specific handling remains here
// rather than in observers such as Recording.
type StreamAssembler interface {
	Add(value any) error
	Finish() (any, error)
}

// NewStreamAssembler adapts the existing protocol assemblers to one common
// interface. Protocol conversion is intentionally out of scope: the caller
// must select the protocol already spoken at its observation boundary.
func NewStreamAssembler(api protocol.APIType) (StreamAssembler, error) {
	switch api {
	case protocol.TypeAnthropicV1:
		return &anthropicV1StreamAssembler{inner: NewAnthropicSDKAssembler()}, nil
	case protocol.TypeAnthropicBeta:
		return &anthropicBetaStreamAssembler{inner: NewAnthropicBetaSDKAssembler()}, nil
	case protocol.TypeOpenAIChat:
		return &openAIChatStreamAssembler{inner: NewOpenAIStreamAssembler()}, nil
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesStreamAssembler{inner: NewResponsesAssembler()}, nil
	default:
		return nil, fmt.Errorf("stream assembler: unsupported protocol %q", api)
	}
}

type anthropicV1StreamAssembler struct {
	inner *AnthropicSDKAssembler
}

func (a *anthropicV1StreamAssembler) Add(value any) error {
	raw, err := streamEventJSON(value)
	if err != nil {
		return err
	}
	var event anthropic.MessageStreamEventUnion
	if err := json.Unmarshal(raw, &event); err != nil {
		return fmt.Errorf("decode Anthropic V1 stream event: %w", err)
	}
	return a.inner.Accumulate(event)
}

func (a *anthropicV1StreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type anthropicBetaStreamAssembler struct {
	inner *AnthropicBetaSDKAssembler
}

func (a *anthropicBetaStreamAssembler) Add(value any) error {
	raw, err := streamEventJSON(value)
	if err != nil {
		return err
	}
	var event anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal(raw, &event); err != nil {
		return fmt.Errorf("decode Anthropic Beta stream event: %w", err)
	}
	return a.inner.Accumulate(event)
}

func (a *anthropicBetaStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type openAIChatStreamAssembler struct {
	inner *OpenAIChatStreamAssembler
}

func (a *openAIChatStreamAssembler) Add(value any) error {
	raw, err := streamEventJSON(value)
	if err != nil {
		return err
	}
	var event openai.ChatCompletionChunk
	if err := json.Unmarshal(raw, &event); err != nil {
		return fmt.Errorf("decode OpenAI Chat stream event: %w", err)
	}
	a.inner.AddChunk(event)
	return nil
}

func (a *openAIChatStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type openAIResponsesStreamAssembler struct {
	inner *ResponsesAssembler
}

func (a *openAIResponsesStreamAssembler) Add(value any) error {
	raw, err := streamEventJSON(value)
	if err != nil {
		return err
	}
	var event responses.ResponseStreamEventUnion
	if err := json.Unmarshal(raw, &event); err != nil {
		return fmt.Errorf("decode OpenAI Responses stream event: %w", err)
	}
	a.inner.Accumulate(event)
	return nil
}

func (a *openAIResponsesStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

func streamEventJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, errors.New("stream event is nil")
	}

	var raw []byte
	switch event := value.(type) {
	case json.RawMessage:
		raw = event
	case []byte:
		raw = event
	case interface{ RawJSON() string }:
		raw = []byte(event.RawJSON())
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("marshal stream event %T: %w", value, err)
			}
		}
	default:
		var err error
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal stream event %T: %w", value, err)
		}
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("stream event %T is not valid JSON", value)
	}
	return append([]byte(nil), raw...), nil
}
