package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// probeEchoInstruction is the system/instruction prompt used by SDK probes to
// keep the upstream response minimal.
const probeEchoInstruction = "work as `echo` if possible"

// extractToolCallInput unmarshals a JSON arguments/input string into a map. A
// missing or invalid JSON body yields an empty map rather than dropping the
// tool call — the name is still useful diagnostic info.
func extractToolCallInput(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

// toolCallsFromOpenAIChat lifts tool calls out of an OpenAI Chat Completions
// response message.
func toolCallsFromOpenAIChat(msg openai.ChatCompletionMessage) []ToolCall {
	var out []ToolCall
	for _, choice := range msg.ToolCalls {
		tc := choice.AsFunction()
		if tc.Function.Name == "" {
			continue
		}
		out = append(out, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: extractToolCallInput(tc.Function.Arguments),
		})
	}
	return out
}

// toolCallsFromOpenAIResponses lifts function-call items out of an OpenAI
// Responses API output list.
func toolCallsFromOpenAIResponses(output []responses.ResponseOutputItemUnion) []ToolCall {
	var out []ToolCall
	for _, item := range output {
		fc := item.AsFunctionCall()
		if fc.Name == "" {
			continue
		}
		out = append(out, ToolCall{
			ID:    fc.ID,
			Name:  fc.Name,
			Input: extractToolCallInput(fc.Arguments),
		})
	}
	return out
}

// toolCallsFromAnthropic lifts tool_use blocks out of an Anthropic Message
// content list.
func toolCallsFromAnthropic(content []anthropic.ContentBlockUnion) []ToolCall {
	var out []ToolCall
	for _, block := range content {
		tu := block.AsToolUse()
		if tu.Name == "" {
			continue
		}
		out = append(out, ToolCall{
			ID:    tu.ID,
			Name:  tu.Name,
			Input: extractToolCallInput(string(tu.Input)),
		})
	}
	return out
}

// The SDK probe helpers below dispatch a minimal request through each client's
// real-traffic methods (ChatCompletionsNew, ResponsesNew, MessagesNew,
// GenerateContent). Routing probes through the same methods as production
// traffic means provider-specific quirks — Kimi model-name normalization,
// Codex Responses handling — apply identically and cannot drift from the real
// path. The client package therefore no longer owns any probe-specific code.

// probeOpenAIChat builds and dispatches a minimal Chat Completions probe.
func probeOpenAIChat(ctx context.Context, oc client.OpenAIClientInterface, model, message string, mode E2EMode) (*Result, error) {
	start := time.Now()
	params := openai.ChatCompletionNewParams{
		Model: model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(probeEchoInstruction),
			openai.UserMessage(message),
		},
	}
	if mode == E2EModeTool {
		params.Tools = getProbeToolsOpenAI()
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.Opt("auto")}
	}
	// Ask for stream usage so the streaming branch can report real token counts.
	// Harmless on the non-streaming path (the provider ignores stream_options).
	params.StreamOptions.IncludeUsage = openai.Opt(true)

	url := oc.GetProvider().APIBase + "/chat/completions"
	if mode == E2EModeSimple {
		resp, err := oc.ChatCompletionsNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		// Tool calls only appear in the message when the request declared tools
		// (tool mode); for simple/streaming probes the slice is empty.
		var toolCalls []ToolCall
		if len(resp.Choices) > 0 {
			toolCalls = toolCallsFromOpenAIChat(resp.Choices[0].Message)
		}
		return toProbeResult(string(b), time.Since(start).Milliseconds(), url, false, usage.FromOpenAIChatCompletion(resp.Usage), toolCalls), nil
	}

	stream := oc.ChatCompletionsNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("chat streaming not supported by provider")
	}
	defer stream.Close()
	var (
		chunks    []any
		streamUse *protocol.TokenUsage
	)
	for stream.Next() {
		ch := stream.Current()
		chunks = append(chunks, ch)
		// OpenAI emits the aggregate Usage on the final (empty-choices) chunk
		// only when stream_options.include_usage is requested; keep the last
		// usage we see so the probe surfaces real token counts.
		if ch.JSON.Usage.Valid() {
			streamUse = usage.FromOpenAIChatCompletion(ch.Usage)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	return toProbeResult(string(b), time.Since(start).Milliseconds(), url, true, streamUse, nil), nil
}

// probeOpenAIResponses builds and dispatches a minimal Responses API probe.
func probeOpenAIResponses(ctx context.Context, oc client.OpenAIClientInterface, model, message string, mode E2EMode) (*Result, error) {
	start := time.Now()
	params := responses.ResponseNewParams{
		Model:        model,
		Instructions: param.NewOpt(probeEchoInstruction),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: []responses.ResponseInputItemUnionParam{
				responses.ResponseInputItemParamOfMessage(
					responses.ResponseInputMessageContentListParam{
						responses.ResponseInputContentParamOfInputText(message),
					},
					responses.EasyInputMessageRoleUser,
				),
			},
		},
	}
	if mode == E2EModeTool {
		params.Tools = getProbeToolsResponses()
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}

	url := oc.GetProvider().APIBase + "/responses"
	if mode == E2EModeSimple {
		resp, err := oc.ResponsesNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return toProbeResult(string(b), time.Since(start).Milliseconds(), url, false, usage.FromOpenAIResponses(resp.Usage), toolCallsFromOpenAIResponses(resp.Output)), nil
	}

	stream := oc.ResponsesNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("responses streaming not supported by provider")
	}
	defer stream.Close()
	var (
		chunks    []any
		streamUse *protocol.TokenUsage
	)
	for stream.Next() {
		ev := stream.Current()
		chunks = append(chunks, ev)
		// The completed Response event carries the aggregate Usage.
		if ev.Type == "response.completed" && ev.Response.Usage.JSON.TotalTokens.Valid() {
			streamUse = usage.FromOpenAIResponses(ev.Response.Usage)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	return toProbeResult(string(b), time.Since(start).Milliseconds(), url, true, streamUse, nil), nil
}

// probeAnthropicMessages builds and dispatches a minimal Messages probe.
func probeAnthropicMessages(ctx context.Context, ac client.AnthropicClientInterface, model, message string, mode E2EMode) (*Result, error) {
	start := time.Now()
	provider := ac.GetProvider()

	system := []anthropic.TextBlockParam{{Text: probeEchoInstruction}}
	if provider.IsClaudeCodeProvider() {
		system = append([]anthropic.TextBlockParam{{Text: client.ClaudeCodeSystemHeader}}, system...)
	}

	params := &anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 1024,
		System:    system,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
	}
	if mode == E2EModeTool {
		params.Tools = getProbeToolsAnthropic()
		params.ToolChoice = getProbeToolChoiceAutoAnthropic()
	}

	url := provider.APIBase + "/v1/messages"
	if mode == E2EModeSimple {
		resp, err := ac.MessagesNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return toProbeResult(string(b), time.Since(start).Milliseconds(), url, false, usage.FromAnthropicMessage(resp.Usage), toolCallsFromAnthropic(resp.Content)), nil
	}

	stream := ac.MessagesNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("messages streaming not supported by provider")
	}
	defer stream.Close()
	acc := usage.NewAnthropicAccumulator()
	var chunks []any
	for stream.Next() {
		ev := stream.Current()
		acc.Consume(&ev)
		chunks = append(chunks, ev)
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	var streamUse *protocol.TokenUsage
	if acc.HasUsage() {
		streamUse = acc.Result()
	}
	return toProbeResult(string(b), time.Since(start).Milliseconds(), url, true, streamUse, nil), nil
}

// probeGoogleGenerate builds and dispatches a minimal GenerateContent probe.
func probeGoogleGenerate(ctx context.Context, gc *client.GoogleClient, model, message string, mode E2EMode) (*Result, error) {
	start := time.Now()
	contents := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: message}}},
	}
	config := &genai.GenerateContentConfig{MaxOutputTokens: 1024}
	url := gc.GetProvider().APIBase

	if mode == E2EModeSimple {
		resp, err := gc.GenerateContent(ctx, model, contents, config)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return toProbeResult(string(b), time.Since(start).Milliseconds(), url, false, nil, nil), nil
	}

	var chunks []any
	for resp, err := range gc.GenerateContentStream(ctx, model, contents, config) {
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, resp)
	}
	b, _ := json.Marshal(chunks)
	return toProbeResult(string(b), time.Since(start).Milliseconds(), url, true, nil, nil), nil
}

// probeOptions issues a bare OPTIONS request to the provider base URL with the
// auth headers appropriate for its API style. Used by the lightweight probe;
// results are advisory.
func probeOptions(ctx context.Context, provider *typ.Provider) Result {
	start := time.Now()

	url := provider.APIBase
	header := http.Header{}
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		apiBase := strings.TrimSuffix(provider.APIBase, "/")
		if !strings.Contains(apiBase, "/v1") {
			apiBase += "/v1"
		}
		url = apiBase
		header.Set("x-api-key", provider.GetAccessToken())
		header.Set("anthropic-version", "2023-06-01")
	case protocol.APIStyleGoogle:
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		header.Set("x-goog-api-key", provider.GetAccessToken())
	default:
		header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, url, nil)
	if err != nil {
		return Result{Success: false, ErrorMessage: fmt.Sprintf("Failed to create OPTIONS request: %v", err)}
	}
	req.Header = header

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return Result{Success: false, ErrorMessage: fmt.Sprintf("OPTIONS request failed: %v", err), LatencyMs: latencyMs}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Result{Success: true, Message: "OPTIONS request successful", LatencyMs: latencyMs}
	}
	return Result{Success: false, ErrorMessage: fmt.Sprintf("OPTIONS request failed with status: %d", resp.StatusCode), LatencyMs: latencyMs}
}
