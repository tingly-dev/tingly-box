package protocoltest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// goSDKClient drives the matrix through the official Go SDKs
// (anthropic-sdk-go, openai-go) instead of hand-crafted JSON. This exercises
// the gateway against real client stacks: strict response unmarshaling, SSE
// consumption via the SDKs' stream iterators, and stream accumulation
// (anthropic.Message.Accumulate / openai.ChatCompletionAccumulator), so a
// response that merely "looks right" semantically but violates the wire
// protocol fails here even when the raw HTTP driver passes.
type goSDKClient struct {
	timeout time.Duration
}

// NewGoSDKClient returns a client driver backed by the official Go SDKs.
func NewGoSDKClient() Client {
	return &goSDKClient{timeout: 30 * time.Second}
}

func (c *goSDKClient) Name() string { return "gosdk" }

func (c *goSDKClient) Supports(source protocol.APIType) bool {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta,
		protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		return true
	}
	return false
}

func (c *goSDKClient) Send(env *TestEnv, spec SendSpec) (*RoundTripResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	switch spec.Source {
	case protocol.TypeAnthropicV1:
		return c.sendAnthropicV1(ctx, spec)
	case protocol.TypeAnthropicBeta:
		return c.sendAnthropicBeta(ctx, spec)
	case protocol.TypeOpenAIChat:
		return c.sendOpenAIChat(ctx, spec)
	case protocol.TypeOpenAIResponses:
		return c.sendOpenAIResponses(ctx, spec)
	default:
		return nil, fmt.Errorf("gosdk client: unsupported source protocol %s", spec.Source)
	}
}

// anthropicClient builds an SDK client pointed at the gateway's Anthropic
// scenario route. The SDK appends /v1/messages to the base URL.
func (c *goSDKClient) anthropicClient(spec SendSpec) anthropic.Client {
	return anthropic.NewClient(
		anthropicoption.WithBaseURL(spec.GatewayURL+"/tingly/anthropic"),
		anthropicoption.WithAPIKey(spec.APIKey),
		anthropicoption.WithMaxRetries(0),
	)
}

func (c *goSDKClient) sendAnthropicV1(ctx context.Context, spec SendSpec) (*RoundTripResult, error) {
	client := c.anthropicClient(spec)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(spec.RequestModel),
		MaxTokens: 1024,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(harnessPrompt))},
	}
	result := newRoundTripResult(spec)

	if !spec.Streaming {
		msg, err := client.Messages.New(ctx, params)
		if err != nil {
			return anthropicErrorResult(result, err)
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(msg.RawJSON())
		normalizeResultJSON(result, result.RawBody, spec.Source, false)
		return result, nil
	}

	stream := client.Messages.NewStreaming(ctx, params)
	msg := anthropic.Message{}
	for stream.Next() {
		ev := stream.Current()
		result.StreamEvents = append(result.StreamEvents, ev.RawJSON())
		if err := msg.Accumulate(ev); err != nil {
			return nil, fmt.Errorf("anthropic SDK accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		if res, rerr := anthropicErrorResult(result, err); rerr == nil {
			return res, nil
		}
		// Mid-stream error (e.g. an in-band error event for a truncated
		// upstream): report it rather than presenting partial content as success.
		result.HTTPStatus = 200
		result.RawBody = []byte(err.Error())
		return result, nil
	}
	result.HTTPStatus = 200
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal accumulated message: %w", err)
	}
	result.RawBody = raw
	normalizeResultJSON(result, raw, spec.Source, true)
	return result, nil
}

func (c *goSDKClient) sendAnthropicBeta(ctx context.Context, spec SendSpec) (*RoundTripResult, error) {
	client := c.anthropicClient(spec)
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(spec.RequestModel),
		MaxTokens: 1024,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(harnessPrompt))},
	}
	// The gateway selects the Beta protocol via the ?beta=true query parameter.
	betaQuery := anthropicoption.WithQueryAdd("beta", "true")
	result := newRoundTripResult(spec)

	if !spec.Streaming {
		msg, err := client.Beta.Messages.New(ctx, params, betaQuery)
		if err != nil {
			return anthropicErrorResult(result, err)
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(msg.RawJSON())
		normalizeResultJSON(result, result.RawBody, spec.Source, false)
		return result, nil
	}

	stream := client.Beta.Messages.NewStreaming(ctx, params, betaQuery)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		ev := stream.Current()
		result.StreamEvents = append(result.StreamEvents, ev.RawJSON())
		if err := msg.Accumulate(ev); err != nil {
			return nil, fmt.Errorf("anthropic beta SDK accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		if res, rerr := anthropicErrorResult(result, err); rerr == nil {
			return res, nil
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(err.Error())
		return result, nil
	}
	result.HTTPStatus = 200
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal accumulated beta message: %w", err)
	}
	result.RawBody = raw
	normalizeResultJSON(result, raw, spec.Source, true)
	return result, nil
}

// openaiClient builds an SDK client pointed at the gateway's OpenAI scenario
// route. The SDK appends chat/completions or responses to the base URL.
func (c *goSDKClient) openaiClient(spec SendSpec) openai.Client {
	return openai.NewClient(
		openaioption.WithBaseURL(spec.GatewayURL+"/tingly/openai/v1/"),
		openaioption.WithAPIKey(spec.APIKey),
		openaioption.WithMaxRetries(0),
	)
}

func (c *goSDKClient) sendOpenAIChat(ctx context.Context, spec SendSpec) (*RoundTripResult, error) {
	client := c.openaiClient(spec)
	params := openai.ChatCompletionNewParams{
		Model:    spec.RequestModel,
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage(harnessPrompt)},
	}
	result := newRoundTripResult(spec)

	if !spec.Streaming {
		resp, err := client.Chat.Completions.New(ctx, params)
		if err != nil {
			return openaiErrorResult(result, err)
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(resp.RawJSON())
		normalizeResultJSON(result, result.RawBody, spec.Source, false)
		return result, nil
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		result.StreamEvents = append(result.StreamEvents, chunk.RawJSON())
		acc.AddChunk(chunk)
	}
	if err := stream.Err(); err != nil {
		if res, rerr := openaiErrorResult(result, err); rerr == nil {
			return res, nil
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(err.Error())
		return result, nil
	}
	result.HTTPStatus = 200
	raw, err := json.Marshal(acc.ChatCompletion)
	if err != nil {
		return nil, fmt.Errorf("marshal accumulated chat completion: %w", err)
	}
	result.RawBody = raw
	normalizeResultJSON(result, raw, spec.Source, true)
	return result, nil
}

func (c *goSDKClient) sendOpenAIResponses(ctx context.Context, spec SendSpec) (*RoundTripResult, error) {
	client := c.openaiClient(spec)
	params := responses.ResponseNewParams{
		Model: spec.RequestModel,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(harnessPrompt)},
	}
	result := newRoundTripResult(spec)

	if !spec.Streaming {
		resp, err := client.Responses.New(ctx, params)
		if err != nil {
			return openaiErrorResult(result, err)
		}
		result.HTTPStatus = 200
		result.RawBody = []byte(resp.RawJSON())
		normalizeResultJSON(result, result.RawBody, spec.Source, false)
		return result, nil
	}

	stream := client.Responses.NewStreaming(ctx, params)
	var sseLines []string
	for stream.Next() {
		ev := stream.Current()
		result.StreamEvents = append(result.StreamEvents, ev.RawJSON())
		sseLines = append(sseLines, "data: "+ev.RawJSON())
	}
	if err := stream.Err(); err != nil {
		if res, rerr := openaiErrorResult(result, err); rerr == nil {
			return res, nil
		}
		if len(result.StreamEvents) == 0 {
			return nil, fmt.Errorf("openai responses SDK stream: %w", err)
		}
	}
	result.HTTPStatus = 200
	// openai-go has no Responses stream accumulator; reuse the harness's SSE
	// assembly over the SDK-parsed events to extract semantics.
	parsed := assembleFromEvents(sseLines, sourceToStyle(spec.Source))
	fillFromParsedResult(result, parsed, sourceToStyle(spec.Source), true)
	if len(sseLines) > 0 {
		result.RawBody = []byte(sseLines[len(sseLines)-1])
	}
	return result, nil
}

// anthropicErrorResult maps an anthropic SDK API error onto the result so
// error-scenario assertions (HTTP status, raw body substring) can run.
// Returns a non-nil error when err is not an API error.
func anthropicErrorResult(result *RoundTripResult, err error) (*RoundTripResult, error) {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return nil, err
	}
	result.HTTPStatus = apiErr.StatusCode
	result.RawBody = apiErr.DumpResponse(true)
	return result, nil
}

// openaiErrorResult is the openai-go counterpart of anthropicErrorResult.
func openaiErrorResult(result *RoundTripResult, err error) (*RoundTripResult, error) {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return nil, err
	}
	result.HTTPStatus = apiErr.StatusCode
	result.RawBody = apiErr.DumpResponse(true)
	return result, nil
}
