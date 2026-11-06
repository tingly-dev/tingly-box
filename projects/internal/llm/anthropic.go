package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicClient struct {
	client anthropic.Client
}

func newAnthropic(conn Connection) *anthropicClient {
	opts := []option.RequestOption{
		// Only the connection configured in settings is used: no
		// ANTHROPIC_* env vars or profiles, and a keyless endpoint (a local
		// gateway without auth) is allowed instead of failing for lack of
		// credentials.
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(strings.TrimRight(conn.BaseURL, "/") + "/"),
		option.WithMaxRetries(1),
	}
	if conn.APIKey != "" {
		opts = append(opts, option.WithAPIKey(conn.APIKey))
	}
	return &anthropicClient{client: anthropic.NewClient(opts...)}
}

func (a *anthropicClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Result, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(maxTokens),
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		var blocks []anthropic.ContentBlockParamUnion
		for _, r := range m.ToolResults {
			blocks = append(blocks, anthropic.NewToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		if m.Content != "" {
			blocks = append(blocks, anthropic.NewTextBlock(m.Content))
		}
		for _, c := range m.ToolCalls {
			blocks = append(blocks, anthropic.NewToolUseBlock(c.ID, json.RawMessage(nonEmptyJSON(c.Input)), c.Name))
		}
		if len(blocks) == 0 {
			continue
		}
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(blocks...))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(blocks...))
		}
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: t.Properties, Required: t.Required},
		}})
	}

	var res Result
	var msg anthropic.Message
	stream := a.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		_ = msg.Accumulate(ev)
		if d, ok := ev.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if t, ok := d.Delta.AsAny().(anthropic.TextDelta); ok && t.Text != "" {
				onDelta(t.Text)
			}
		}
	}
	var text strings.Builder
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			res.ToolCalls = append(res.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Input: json.RawMessage(nonEmptyJSON(b.Input))})
		}
	}
	res.Text = text.String()
	res.StopReason = string(msg.StopReason)
	res.Usage = Usage{InputTokens: int(msg.Usage.InputTokens), OutputTokens: int(msg.Usage.OutputTokens)}
	return res, stream.Err()
}

// nonEmptyJSON returns raw, or "{}" when it is empty.
func nonEmptyJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func (a *anthropicClient) ListModels(ctx context.Context) ([]Model, error) {
	var out []Model
	pager := a.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for pager.Next() {
		m := pager.Current()
		out = append(out, Model{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out, pager.Err()
}
