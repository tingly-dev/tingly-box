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
	Terminal() bool
	TerminalError() error
}

// NewStreamAssembler adapts the existing protocol assemblers to one common
// interface. Protocol conversion is intentionally out of scope: the caller
// must select the protocol already spoken at its observation boundary.
func NewStreamAssembler(api protocol.APIType) (StreamAssembler, error) {
	return newStreamAssembler(api, 1)
}

// NewStreamAssemblerForRequest configures protocol-specific expectations from
// the provider-bound request. Today this is used for OpenAI Chat's n choices.
func NewStreamAssemblerForRequest(api protocol.APIType, request any) (StreamAssembler, error) {
	return newStreamAssembler(api, OpenAIChatChoiceCount(request))
}

func newStreamAssembler(api protocol.APIType, expectedChatChoices int) (StreamAssembler, error) {
	switch api {
	case protocol.TypeAnthropicV1:
		return &anthropicV1StreamAssembler{inner: NewAnthropicSDKAssembler()}, nil
	case protocol.TypeAnthropicBeta:
		return &anthropicBetaStreamAssembler{inner: NewAnthropicBetaSDKAssembler()}, nil
	case protocol.TypeOpenAIChat:
		return &openAIChatStreamAssembler{
			inner:           NewOpenAIStreamAssembler(),
			expectedChoices: expectedChatChoices,
		}, nil
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesStreamAssembler{inner: NewResponsesAssembler()}, nil
	default:
		return nil, fmt.Errorf("stream assembler: unsupported protocol %q", api)
	}
}

// OpenAIChatChoiceCount returns the request's expected number of choices,
// defaulting invalid, omitted, or non-Chat values to one.
func OpenAIChatChoiceCount(request any) int {
	chatRequest, ok := request.(*openai.ChatCompletionNewParams)
	if !ok || chatRequest == nil {
		return 1
	}
	choices := chatRequest.N.Or(1)
	if choices < 1 {
		return 1
	}
	maxInt := int(^uint(0) >> 1)
	if uint64(choices) > uint64(maxInt) {
		return maxInt
	}
	return int(choices)
}

type anthropicV1StreamAssembler struct {
	inner    *AnthropicSDKAssembler
	started  bool
	terminal bool
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
	if err := a.inner.Accumulate(event); err != nil {
		return err
	}
	if event.Type == "message_start" {
		a.started = true
	}
	if event.Type == "message_stop" && a.started {
		a.terminal = true
	}
	return nil
}

func (a *anthropicV1StreamAssembler) Terminal() bool     { return a.terminal }
func (*anthropicV1StreamAssembler) TerminalError() error { return nil }

func (a *anthropicV1StreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type anthropicBetaStreamAssembler struct {
	inner    *AnthropicBetaSDKAssembler
	started  bool
	terminal bool
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
	if err := a.inner.Accumulate(event); err != nil {
		return err
	}
	if event.Type == "message_start" {
		a.started = true
	}
	if event.Type == "message_stop" && a.started {
		a.terminal = true
	}
	return nil
}

func (a *anthropicBetaStreamAssembler) Terminal() bool     { return a.terminal }
func (*anthropicBetaStreamAssembler) TerminalError() error { return nil }

func (a *anthropicBetaStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type openAIChatStreamAssembler struct {
	inner           *OpenAIChatStreamAssembler
	expectedChoices int
	finishedChoices map[int64]struct{}
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
	if !a.inner.AddChunk(event) {
		return fmt.Errorf("accumulate OpenAI Chat stream chunk %q", event.ID)
	}
	for _, choice := range event.Choices {
		if choice.FinishReason != "" && choice.Index >= 0 && choice.Index < int64(a.expectedChoices) {
			if a.finishedChoices == nil {
				a.finishedChoices = make(map[int64]struct{})
			}
			a.finishedChoices[choice.Index] = struct{}{}
		}
	}
	return nil
}

func (a *openAIChatStreamAssembler) Terminal() bool {
	return len(a.finishedChoices) >= a.expectedChoices
}
func (*openAIChatStreamAssembler) TerminalError() error { return nil }

func (a *openAIChatStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

type openAIResponsesStreamAssembler struct {
	inner       *ResponsesAssembler
	terminalErr error
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
	switch event.Type {
	case "response.failed", "error":
		a.terminalErr = fmt.Errorf("OpenAI Responses stream ended with %s", event.Type)
	}
	return nil
}

func (a *openAIResponsesStreamAssembler) Finish() (any, error) {
	return a.inner.Finish(), nil
}

func (a *openAIResponsesStreamAssembler) Terminal() bool       { return a.inner.IsFinished() }
func (a *openAIResponsesStreamAssembler) TerminalError() error { return a.terminalErr }

func streamEventJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, errors.New("stream event is nil")
	}

	raw, err := protocol.SnapshotJSON(value)
	if err != nil {
		return nil, fmt.Errorf("snapshot stream event %T: %w", value, err)
	}
	return raw, nil
}
