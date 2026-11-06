package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type openaiClient struct {
	client openai.Client
}

func newOpenAI(conn Connection) *openaiClient {
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(conn.BaseURL, "/") + "/"),
		option.WithMaxRetries(1),
		// Never fall back to OPENAI_API_KEY from the environment.
		option.WithAPIKey(conn.APIKey),
	}
	return &openaiClient{client: openai.NewClient(opts...)}
}

func (o *openaiClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Result, error) {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		for _, r := range m.ToolResults {
			msgs = append(msgs, openai.ToolMessage(r.Content, r.CallID))
		}
		if m.Role == "assistant" {
			a := openai.ChatCompletionAssistantMessageParam{}
			if m.Content != "" {
				a.Content.OfString = openai.String(m.Content)
			}
			for _, c := range m.ToolCalls {
				a.ToolCalls = append(a.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID:       c.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: string(nonEmptyJSON(c.Input))},
					},
				})
			}
			if m.Content != "" || len(a.ToolCalls) > 0 {
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &a})
			}
		} else if m.Content != "" {
			msgs = append(msgs, openai.UserMessage(m.Content))
		}
	}
	params := openai.ChatCompletionNewParams{
		Model:         openai.ChatModel(req.Model),
		Messages:      msgs,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  openai.FunctionParameters{"type": "object", "properties": t.Properties, "required": t.Required},
		}))
	}

	var res Result
	var acc openai.ChatCompletionAccumulator
	stream := o.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				onDelta(ch.Delta.Content)
			}
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			res.Usage = Usage{InputTokens: int(chunk.Usage.PromptTokens), OutputTokens: int(chunk.Usage.CompletionTokens)}
		}
	}
	if len(acc.Choices) > 0 {
		ch := acc.Choices[0]
		res.Text = ch.Message.Content
		res.StopReason = ch.FinishReason
		for _, c := range ch.Message.ToolCalls {
			res.ToolCalls = append(res.ToolCalls, ToolCall{ID: c.ID, Name: c.Function.Name, Input: json.RawMessage(nonEmptyJSON([]byte(c.Function.Arguments)))})
		}
	}
	return res, stream.Err()
}

func (o *openaiClient) ListModels(ctx context.Context) ([]Model, error) {
	var out []Model
	pager := o.client.Models.ListAutoPaging(ctx)
	for pager.Next() {
		m := pager.Current()
		out = append(out, Model{ID: m.ID, DisplayName: m.ID})
	}
	return out, pager.Err()
}
