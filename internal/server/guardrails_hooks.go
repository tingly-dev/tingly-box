package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// GuardrailsHookResult holds the final evaluation result.
type GuardrailsHookResult struct {
	Result guardrails.Result
	Err    error
}

// GuardrailsHookOption customizes guardrails hook behavior.
type GuardrailsHookOption func(*guardrailsHook)

type guardrailsHook struct {
	engine    guardrails.Guardrails
	baseInput guardrails.Input
	ctx       context.Context
	onVerdict func(GuardrailsHookResult)
	acc       *guardrailsAccumulator
	mu        sync.Mutex
}

// WithGuardrailsContext sets the context used for evaluation.
func WithGuardrailsContext(ctx context.Context) GuardrailsHookOption {
	return func(h *guardrailsHook) {
		if ctx != nil {
			h.ctx = ctx
		}
	}
}

// WithGuardrailsOnVerdict registers a callback for results.
func WithGuardrailsOnVerdict(cb func(GuardrailsHookResult)) GuardrailsHookOption {
	return func(h *guardrailsHook) {
		h.onVerdict = cb
	}
}

// NewGuardrailsHooks creates stream hooks that evaluate guardrails on completion.
// baseInput can include scenario/model/tags/metadata and initial content/messages.
func NewGuardrailsHooks(engine guardrails.Guardrails, baseInput guardrails.Input, opts ...GuardrailsHookOption) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if engine == nil {
		return nil, nil, nil
	}

	hook := &guardrailsHook{
		engine:    engine,
		baseInput: baseInput,
		ctx:       context.Background(),
		acc:       &guardrailsAccumulator{},
	}
	for _, opt := range opts {
		opt(hook)
	}

	onStreamEvent = func(event interface{}) error {
		hook.mu.Lock()
		defer hook.mu.Unlock()

		switch evt := event.(type) {
		case *openai.ChatCompletionChunk:
			hook.acc.ingestOpenAIChatChunk(evt)
		case *responses.ResponseStreamEventUnion:
			hook.acc.ingestOpenAIResponseEvent(evt)
		case map[string]interface{}:
			hook.acc.ingestMapEvent(evt)
		default:
			hook.acc.ingestAnyEvent(evt)
		}
		return nil
	}

	onStreamComplete = func() {
		hook.mu.Lock()
		input := hook.buildInputLocked()
		ctx := hook.ctx
		onVerdict := hook.onVerdict
		scenario := input.Scenario
		model := input.Model
		hook.mu.Unlock()

		logrus.Debugf("Guardrails: evaluating stream completion (scenario=%s model=%s)", scenario, model)
		result, err := hook.engine.Evaluate(ctx, input)
		if err != nil {
			logrus.Debugf("Guardrails: evaluation error (scenario=%s model=%s): %v", scenario, model, err)
		} else {
			logrus.Debugf("Guardrails: evaluation done (scenario=%s model=%s verdict=%s)", scenario, model, result.Verdict)
		}
		if onVerdict != nil {
			onVerdict(GuardrailsHookResult{Result: result, Err: err})
		}
	}

	onStreamError = func(err error) {
		logrus.Debugf("Guardrails: stream error before evaluation: %v", err)
		if hook.onVerdict != nil {
			hook.onVerdict(GuardrailsHookResult{Err: err})
		}
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

// NewNonStreamGuardrailsHook evaluates guardrails for non-streaming responses.
func NewNonStreamGuardrailsHook(engine guardrails.Guardrails, input guardrails.Input, opts ...GuardrailsHookOption) func() {
	if engine == nil {
		return nil
	}

	hook := &guardrailsHook{
		engine:    engine,
		baseInput: input,
		ctx:       context.Background(),
	}
	for _, opt := range opts {
		opt(hook)
	}

	return func() {
		logrus.Debugf("Guardrails: evaluating non-stream input (scenario=%s model=%s)", hook.baseInput.Scenario, hook.baseInput.Model)
		result, err := hook.engine.Evaluate(hook.ctx, hook.baseInput)
		if err != nil {
			logrus.Debugf("Guardrails: non-stream evaluation error (scenario=%s model=%s): %v", hook.baseInput.Scenario, hook.baseInput.Model, err)
		} else {
			logrus.Debugf("Guardrails: non-stream evaluation done (scenario=%s model=%s verdict=%s)", hook.baseInput.Scenario, hook.baseInput.Model, result.Verdict)
		}
		if hook.onVerdict != nil {
			hook.onVerdict(GuardrailsHookResult{Result: result, Err: err})
		}
	}
}

func (h *guardrailsHook) buildInputLocked() guardrails.Input {
	input := h.baseInput
	if input.Direction == "" {
		input.Direction = guardrails.DirectionResponse
	}

	content := input.Content
	accContent := h.acc.content()

	if content.Text == "" {
		content.Text = accContent.Text
	} else if accContent.Text != "" {
		content.Text = strings.TrimRight(content.Text, "\n") + "\n" + accContent.Text
	}

	if content.Command == nil {
		content.Command = accContent.Command
	}

	input.Content = content
	return input
}

type guardrailsAccumulator struct {
	textBuilder  strings.Builder
	commandName  string
	commandArgs  strings.Builder
	commandFound bool
}

func (a *guardrailsAccumulator) ingestOpenAIChatChunk(chunk *openai.ChatCompletionChunk) {
	if chunk == nil || len(chunk.Choices) == 0 {
		return
	}
	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		a.textBuilder.WriteString(choice.Delta.Content)
	}
	if choice.Delta.FunctionCall.Name != "" || choice.Delta.FunctionCall.Arguments != "" {
		if choice.Delta.FunctionCall.Name != "" {
			a.commandName = choice.Delta.FunctionCall.Name
			a.commandFound = true
		}
		if choice.Delta.FunctionCall.Arguments != "" {
			a.commandArgs.WriteString(choice.Delta.FunctionCall.Arguments)
			a.commandFound = true
		}
	}
	for _, toolCall := range choice.Delta.ToolCalls {
		if toolCall.Function.Name != "" {
			a.commandName = toolCall.Function.Name
			a.commandFound = true
		}
		if toolCall.Function.Arguments != "" {
			a.commandArgs.WriteString(toolCall.Function.Arguments)
			a.commandFound = true
		}
	}
}

func (a *guardrailsAccumulator) ingestOpenAIResponseEvent(evt *responses.ResponseStreamEventUnion) {
	if evt == nil {
		return
	}
	a.ingestRawJSON(evt.RawJSON())
}

func (a *guardrailsAccumulator) ingestMapEvent(event map[string]interface{}) {
	if event == nil {
		return
	}
	a.ingestEventMap(event)
}

func (a *guardrailsAccumulator) ingestAnyEvent(event interface{}) {
	if event == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	a.ingestEventMap(payload)
}

func (a *guardrailsAccumulator) ingestRawJSON(raw string) {
	if raw == "" {
		return
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	a.ingestEventMap(payload)
}

func (a *guardrailsAccumulator) ingestEventMap(payload map[string]interface{}) {
	eventType, _ := payload["type"].(string)

	switch eventType {
	case "content_block_delta":
		delta, _ := payload["delta"].(map[string]interface{})
		a.ingestDelta(delta)
	case "content_block_start":
		block, _ := payload["content_block"].(map[string]interface{})
		a.ingestContentBlock(block)
	case "response.output_text.delta":
		if delta, ok := payload["delta"].(string); ok {
			a.textBuilder.WriteString(delta)
		}
	case "response.output_text.done":
		if text, ok := payload["text"].(string); ok {
			a.textBuilder.WriteString(text)
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.mcp_call_arguments.delta":
		if delta, ok := payload["delta"].(string); ok {
			a.commandArgs.WriteString(delta)
			a.commandFound = true
		}
	case "response.function_call_arguments.done", "response.custom_tool_call_input.done", "response.mcp_call_arguments.done":
		if name, ok := payload["name"].(string); ok && name != "" {
			a.commandName = name
			a.commandFound = true
		}
	case "response.output_item.added":
		item, _ := payload["item"].(map[string]interface{})
		a.ingestOutputItem(item)
	case "response.completed":
		response, _ := payload["response"].(map[string]interface{})
		if output, ok := response["output"].([]interface{}); ok {
			for _, item := range output {
				if itemMap, ok := item.(map[string]interface{}); ok {
					a.ingestOutputItem(itemMap)
				}
			}
		}
	}
}

func (a *guardrailsAccumulator) ingestDelta(delta map[string]interface{}) {
	if delta == nil {
		return
	}
	deltaType, _ := delta["type"].(string)
	switch deltaType {
	case "text_delta":
		if text, ok := delta["text"].(string); ok {
			a.textBuilder.WriteString(text)
		}
	case "input_json_delta":
		if partial, ok := delta["partial_json"].(string); ok {
			a.commandArgs.WriteString(partial)
			a.commandFound = true
		}
	}
}

func (a *guardrailsAccumulator) ingestContentBlock(block map[string]interface{}) {
	if block == nil {
		return
	}
	blockType, _ := block["type"].(string)
	if blockType != "tool_use" && blockType != "function_call" {
		return
	}
	if name, ok := block["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
	}
	if input, ok := block["input"].(map[string]interface{}); ok {
		payload, err := json.Marshal(input)
		if err == nil {
			a.commandArgs.Write(payload)
			a.commandFound = true
		}
	}
}

func (a *guardrailsAccumulator) ingestOutputItem(item map[string]interface{}) {
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	if itemType != "function_call" && itemType != "custom_tool_call" && itemType != "mcp_call" {
		return
	}
	if name, ok := item["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
	}
	if args, ok := item["arguments"].(string); ok && args != "" {
		a.commandArgs.WriteString(args)
		a.commandFound = true
	}
	if input, ok := item["input"].(string); ok && input != "" {
		a.commandArgs.WriteString(input)
		a.commandFound = true
	}
}

func (a *guardrailsAccumulator) content() guardrails.Content {
	text := strings.TrimSpace(a.textBuilder.String())

	content := guardrails.Content{Text: text}
	if !a.commandFound {
		return content
	}

	cmd := &guardrails.Command{Name: a.commandName}
	args := strings.TrimSpace(a.commandArgs.String())
	if args != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(args), &parsed); err == nil {
			cmd.Arguments = parsed
		} else {
			cmd.Arguments = map[string]interface{}{"_raw": args}
		}
	}

	content.Command = cmd
	return content
}

func guardrailsMessagesFromAnthropicV1(system []anthropic.TextBlockParam, messages []anthropic.MessageParam) []guardrails.Message {
	out := make([]guardrails.Message, 0, len(messages)+1)

	if len(system) > 0 {
		out = append(out, guardrails.Message{
			Role:    "system",
			Content: request.ConvertTextBlocksToString(system),
		})
	}

	for _, msg := range messages {
		content := request.ConvertContentBlocksToString(msg.Content)
		out = append(out, guardrails.Message{
			Role:    string(msg.Role),
			Content: content,
		})
	}

	return out
}

func guardrailsMessagesFromAnthropicV1Beta(system []anthropic.BetaTextBlockParam, messages []anthropic.BetaMessageParam) []guardrails.Message {
	out := make([]guardrails.Message, 0, len(messages)+1)

	if len(system) > 0 {
		out = append(out, guardrails.Message{
			Role:    "system",
			Content: request.ConvertBetaTextBlocksToString(system),
		})
	}

	for _, msg := range messages {
		content := request.ConvertBetaContentBlocksToString(msg.Content)
		out = append(out, guardrails.Message{
			Role:    string(msg.Role),
			Content: content,
		})
	}

	return out
}
