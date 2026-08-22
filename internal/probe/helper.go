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
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// probeEchoInstruction is the system/instruction prompt used by SDK probes to
// keep the upstream response minimal.
const probeEchoInstruction = "work as `echo` if possible"

// probeParams carries the fully-resolved request shape handed down from
// Probe() -> probeProviderWithSDK() -> the SDK helpers. The caller resolves
// the wire Stream/Tool axes into flat booleans once at the entry point, so
// the helpers never branch on the request — they read flat decisions: use
// the streaming path? attach tools? enable thinking? Adding a future knob
// means a field here, not a new branch inside every helper.
type probeParams struct {
	Model    string
	Message  string
	Stream   bool // true → take the SSE round-trip; false → single response
	Tool     bool // true → attach probe tools + auto tool_choice (tool mode)
	Thinking ThinkingLevel
	Vision   VisionChannel // user/tool → attach the canonical vision fixture turn
}

// thinkingEnabled reports whether the probe should enable extended thinking.
// "" and "none" both mean "send no thinking param" (the default).
func thinkingEnabled(t ThinkingLevel) bool {
	return t != "" && t != ThinkingNone
}

// thinkingBudget returns the budget_tokens for the given level, via the
// canonical thinking.BudgetMapping. Used by the budget-dialect targets
// (Anthropic thinking.budget_tokens, Gemini thinking_budget).
func thinkingBudget(t ThinkingLevel) int64 {
	return thinking.BudgetMapping[t]
}

// thinkingEffort returns the effort string for effort-based targets
// (OpenAI reasoning_effort / Responses reasoning.effort). The probe's ladder
// values (low/medium/high) are already valid effort strings; this helper keeps
// the call sites readable.
func thinkingEffort(t ThinkingLevel) shared.ReasoningEffort {
	return shared.ReasoningEffort(t)
}

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

// buildOpenAIChatParams assembles the Chat Completions request params for a
// probe. Shared by the probe helper and the cURL builder so the constructed
// request cannot drift between the two paths.
func buildOpenAIChatParams(p probeParams) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    p.Model,
		Messages: buildOpenAIChatVisionMessages(p),
	}
	if p.Tool {
		params.Tools = getProbeToolsOpenAI()
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.Opt("auto")}
	}
	if p.Vision == VisionTool {
		params.Tools = append(params.Tools, getVisionToolOpenAI())
	}
	if thinkingEnabled(p.Thinking) {
		params.ReasoningEffort = thinkingEffort(p.Thinking)
	}
	if p.Stream {
		// stream_options is valid only for streaming requests; the SDK adds
		// the "stream": true member itself at request time (WithJSONSet).
		params.StreamOptions.IncludeUsage = openai.Opt(true)
	}
	return params
}

// buildOpenAIChatVisionMessages returns the Chat message list for the probe's
// vision channel. The echo system instruction is dropped for vision probes —
// the fixture prompt ("what color…") is the diagnostic, and an echoing model
// would answer with the question instead of the color.
func buildOpenAIChatVisionMessages(p probeParams) []openai.ChatCompletionMessageParamUnion {
	switch p.Vision {
	case VisonUser:
		return []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				{OfText: &openai.ChatCompletionContentPartTextParam{Text: visionproxy.Prompt}},
				{OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: visionproxy.FixtureDataURL},
				}},
			}),
		}
	case VisionTool:
		return []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(visionproxy.ToolUserText),
			{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
					{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: visionproxy.ToolCallID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      visionproxy.ToolName,
							Arguments: "{}",
						},
					}},
				},
			}},
			openai.ToolMessage([]openai.ChatCompletionContentPartUnionParam{
				{OfText: &openai.ChatCompletionContentPartTextParam{Text: visionproxy.ToolResultText}},
				{OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: visionproxy.FixtureDataURL},
				}},
			}, visionproxy.ToolCallID),
		}
	default:
		return []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(probeEchoInstruction),
			openai.UserMessage(p.Message),
		}
	}
}

// probeOpenAIChat builds and dispatches a minimal Chat Completions probe.
func probeOpenAIChat(ctx context.Context, oc client.OpenAIClientInterface, p probeParams) (*Result, error) {
	start := time.Now()
	params := buildOpenAIChatParams(p)
	url := oc.GetProvider().APIBase + "/chat/completions"
	// Tool mode needs the structured tool_calls back out of the response, so
	// it takes the non-streaming path alongside simple mode; only streaming
	// mode itself needs the SSE round-trip.
	if !p.Stream {
		resp, err := oc.ChatCompletionsNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		// Tool calls only appear in the message when the request declared tools
		// (tool mode); for simple probes the slice is empty.
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

// buildOpenAIResponsesParams assembles the Responses API request params for a
// probe. Shared by the probe helper and the cURL builder.
func buildOpenAIResponsesParams(p probeParams) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: p.Model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: buildOpenAIResponsesVisionInput(p),
		},
	}
	if !p.Vision.Enabled() {
		// Echo instruction only for non-vision probes — the vision fixture
		// prompt is the diagnostic and must not be echoed back.
		params.Instructions = param.NewOpt(probeEchoInstruction)
	}
	if p.Tool {
		params.Tools = getProbeToolsResponses()
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}
	if p.Vision == VisionTool {
		params.Tools = append(params.Tools, getVisionToolResponses())
	}
	if thinkingEnabled(p.Thinking) {
		params.Reasoning.Effort = thinkingEffort(p.Thinking)
	}
	return params
}

// buildOpenAIResponsesVisionInput returns the Responses input item list for
// the probe's vision channel.
func buildOpenAIResponsesVisionInput(p probeParams) []responses.ResponseInputItemUnionParam {
	switch p.Vision {
	case VisonUser:
		return []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(
				responses.ResponseInputMessageContentListParam{
					responses.ResponseInputContentParamOfInputText(visionproxy.Prompt),
					{OfInputImage: &responses.ResponseInputImageParam{
						ImageURL: param.NewOpt(visionproxy.FixtureDataURL),
					}},
				},
				responses.EasyInputMessageRoleUser,
			),
		}
	case VisionTool:
		return []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(
				responses.ResponseInputMessageContentListParam{
					responses.ResponseInputContentParamOfInputText(visionproxy.ToolUserText),
				},
				responses.EasyInputMessageRoleUser,
			),
			{OfFunctionCall: &responses.ResponseFunctionToolCallParam{
				CallID:    visionproxy.ToolCallID,
				Name:      visionproxy.ToolName,
				Arguments: "{}",
			}},
			{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				CallID: visionproxy.ToolCallID,
				Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
					OfResponseFunctionCallOutputItemArray: responses.ResponseFunctionCallOutputItemListParam{
						{OfInputText: &responses.ResponseInputTextContentParam{Text: visionproxy.ToolResultText}},
						{OfInputImage: &responses.ResponseInputImageContentParam{
							ImageURL: param.NewOpt(visionproxy.FixtureDataURL),
						}},
					},
				},
			}},
		}
	default:
		return []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfMessage(
				responses.ResponseInputMessageContentListParam{
					responses.ResponseInputContentParamOfInputText(p.Message),
				},
				responses.EasyInputMessageRoleUser,
			),
		}
	}
}

// probeOpenAIResponses builds and dispatches a minimal Responses API probe.
func probeOpenAIResponses(ctx context.Context, oc client.OpenAIClientInterface, p probeParams) (*Result, error) {
	start := time.Now()
	params := buildOpenAIResponsesParams(p)

	url := oc.GetProvider().APIBase + "/responses"
	if !p.Stream {
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

// buildAnthropicMessageParams assembles the Messages request params for a
// probe. Shared by the probe helper and the cURL builder. The SDK adds the
// "stream": true member itself at request time (WithJSONSet).
func buildAnthropicMessageParams(p probeParams, isClaudeCodeProvider bool) *anthropic.MessageNewParams {
	var system []anthropic.TextBlockParam
	if !p.Vision.Enabled() {
		// Echo instruction only for non-vision probes — the vision fixture
		// prompt is the diagnostic and must not be echoed back.
		system = []anthropic.TextBlockParam{{Text: probeEchoInstruction}}
	}
	if isClaudeCodeProvider {
		system = append([]anthropic.TextBlockParam{{Text: client.ClaudeCodeSystemHeader}}, system...)
	}

	params := &anthropic.MessageNewParams{
		Model:     anthropic.Model(p.Model),
		MaxTokens: 1024,
		System:    system,
		Messages:  buildAnthropicVisionMessages(p),
	}
	if p.Tool {
		params.Tools = getProbeToolsAnthropic()
		params.ToolChoice = getProbeToolChoiceAutoAnthropic()
	}
	if p.Vision == VisionTool {
		params.Tools = append(params.Tools, getVisionToolAnthropic())
	}
	if thinkingEnabled(p.Thinking) {
		// Extended thinking requires 1024 <= budget_tokens < max_tokens, so
		// raise MaxTokens above the budget. +2048 leaves headroom for the
		// visible answer beyond the thinking budget.
		budget := thinkingBudget(p.Thinking)
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: budget},
		}
		params.MaxTokens = budget + 2048
	}
	return params
}

// buildAnthropicVisionMessages returns the Messages list for the probe's
// vision channel.
func buildAnthropicVisionMessages(p probeParams) []anthropic.MessageParam {
	switch p.Vision {
	case VisonUser:
		return []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(visionproxy.Prompt),
				anthropic.NewImageBlockBase64(visionproxy.FixtureMediaType, visionproxy.FixturePNGBase64),
			),
		}
	case VisionTool:
		return []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(visionproxy.ToolUserText)),
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewToolUseBlock(visionproxy.ToolCallID, map[string]any{}, visionproxy.ToolName),
				},
			},
			anthropic.NewUserMessage(anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: visionproxy.ToolCallID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: visionproxy.ToolResultText}},
						{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfBase64: &anthropic.Base64ImageSourceParam{
									MediaType: visionproxy.FixtureMediaType,
									Data:      visionproxy.FixturePNGBase64,
								},
							},
						}},
					},
				},
			}),
		}
	default:
		return []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(p.Message)),
		}
	}
}

// probeAnthropicMessages builds and dispatches a minimal Messages probe.
func probeAnthropicMessages(ctx context.Context, ac client.AnthropicClientInterface, p probeParams) (*Result, error) {
	start := time.Now()
	provider := ac.GetProvider()

	params := buildAnthropicMessageParams(p, provider.IsClaudeCodeProvider())

	url := provider.APIBase + "/v1/messages"
	if !p.Stream {
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
func probeGoogleGenerate(ctx context.Context, gc *client.GoogleClient, p probeParams) (*Result, error) {
	// The Google SDK path has no loopback and no vision fixture mapping —
	// refuse explicitly rather than silently probing without the image.
	if p.Vision.Enabled() {
		return nil, fmt.Errorf("vision probes are not supported for Google-style providers")
	}
	start := time.Now()
	contents := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: p.Message}}},
	}
	config := &genai.GenerateContentConfig{MaxOutputTokens: 1024}
	url := gc.GetProvider().APIBase

	if thinkingEnabled(p.Thinking) {
		budget := int32(thinkingBudget(p.Thinking))
		config.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &budget}
	}

	if !p.Stream {
		resp, err := gc.GenerateContent(ctx, p.Model, contents, config)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return toProbeResult(string(b), time.Since(start).Milliseconds(), url, false, nil, nil), nil
	}

	var chunks []any
	for resp, err := range gc.GenerateContentStream(ctx, p.Model, contents, config) {
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
