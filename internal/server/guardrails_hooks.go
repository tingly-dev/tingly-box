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

// GuardrailsHookResult holds the final evaluation result and optional block metadata.
type GuardrailsHookResult struct {
	Result guardrails.Result
	Err    error
	// BlockMessage is an optional message to inject when verdict is block.
	BlockMessage string
	// BlockIndex is the suggested content block index for Anthropic streams.
	BlockIndex int
	// BlockToolID is the associated tool_use id for tool_result injection.
	BlockToolID string
}

// GuardrailsHookOption customizes guardrails hook behavior.
type GuardrailsHookOption func(*guardrailsHook)

type guardrailsHook struct {
	engine    guardrails.Guardrails
	baseInput guardrails.Input
	ctx       context.Context
	onVerdict func(GuardrailsHookResult)
	onBlock   func(GuardrailsHookResult)
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

// WithGuardrailsOnBlock registers a callback for early tool_use block decisions.
func WithGuardrailsOnBlock(cb func(GuardrailsHookResult)) GuardrailsHookOption {
	return func(h *guardrailsHook) {
		h.onBlock = cb
	}
}

// NewGuardrailsHooks creates stream hooks that evaluate guardrails on completion.
// baseInput can include scenario/model/tags/metadata and initial content/messages.
// WithGuardrailsOnBlock enables early tool_use blocking during the stream.
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
		case *anthropic.MessageStreamEventUnion:
			hook.acc.ingestAnthropicEvent(evt)
		case *anthropic.BetaRawMessageStreamEventUnion:
			hook.acc.ingestAnthropicBetaEvent(evt)
		case *openai.ChatCompletionChunk:
			hook.acc.ingestOpenAIChatChunk(evt)
		case *responses.ResponseStreamEventUnion:
			hook.acc.ingestOpenAIResponseEvent(evt)
		case map[string]interface{}:
			hook.acc.ingestMapEvent(evt)
		default:
			hook.acc.ingestAnyEvent(evt)
		}

		if hook.onBlock != nil {
			if toolUse, ok := hook.acc.popCompletedToolUse(); ok {
				input := hook.baseInput
				input.Direction = guardrails.DirectionResponse
				input.Content = guardrails.Content{
					Messages: input.Content.Messages,
					Command: &guardrails.Command{
						Name:      toolUse.name,
						Arguments: parseToolArgs(toolUse.args),
					},
				}
				result, err := hook.engine.Evaluate(hook.ctx, input)
				if err == nil && result.Verdict == guardrails.VerdictBlock {
					hook.onBlock(GuardrailsHookResult{
						Result:       result,
						BlockMessage: guardrailsBlockMessageForCommand(result, toolUse.name, parseToolArgs(toolUse.args)),
						BlockIndex:   toolUse.index,
						BlockToolID:  toolUse.id,
					})
				}
			}
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
		blockIndex := hook.acc.nextBlockIndex()
		blockToolID := hook.acc.lastToolID
		hook.mu.Unlock()

		logrus.Debugf("Guardrails: evaluating stream completion (scenario=%s model=%s)", scenario, model)
		result, err := hook.engine.Evaluate(ctx, input)
		if err != nil {
			logrus.Debugf("Guardrails: evaluation error (scenario=%s model=%s): %v", scenario, model, err)
		} else {
			logrus.Debugf("Guardrails: evaluation done (scenario=%s model=%s verdict=%s)", scenario, model, result.Verdict)
		}
		if onVerdict != nil {
			blockMsg := ""
			if result.Verdict == guardrails.VerdictBlock {
				blockMsg = guardrailsBlockMessageWithSnippet(result, input.Content.Preview(120))
			}
			onVerdict(GuardrailsHookResult{
				Result:       result,
				Err:          err,
				BlockMessage: blockMsg,
				BlockIndex:   blockIndex,
				BlockToolID:  blockToolID,
			})
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
	lastIndex    int
	hasIndex     bool
	lastToolID   string
	toolUses     map[int]*toolUseState
	completed    []toolUseState
}

type toolUseState struct {
	index int
	id    string
	name  string
	args  string
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

func (a *guardrailsAccumulator) ingestAnthropicEvent(evt *anthropic.MessageStreamEventUnion) {
	if evt == nil {
		return
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}
	data["type"] = evt.Type
	a.ingestEventMap(data)
}

func (a *guardrailsAccumulator) ingestAnthropicBetaEvent(evt *anthropic.BetaRawMessageStreamEventUnion) {
	if evt == nil {
		return
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}
	data["type"] = evt.Type
	a.ingestEventMap(data)
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
	index := a.captureIndex(payload)

	switch eventType {
	case "content_block_delta":
		delta, _ := payload["delta"].(map[string]interface{})
		a.ingestDelta(index, delta)
	case "content_block_start":
		block, _ := payload["content_block"].(map[string]interface{})
		a.ingestContentBlock(index, block)
	case "content_block_stop":
		a.ingestContentBlockStop(index)
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

func (a *guardrailsAccumulator) ingestDelta(index int, delta map[string]interface{}) {
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
			if state := a.getOrCreateToolUse(index); state != nil {
				state.args += partial
			}
		}
	}
}

func (a *guardrailsAccumulator) ingestContentBlock(index int, block map[string]interface{}) {
	if block == nil {
		return
	}
	blockType, _ := block["type"].(string)
	if blockType != "tool_use" && blockType != "function_call" {
		return
	}
	if id, ok := block["id"].(string); ok && id != "" {
		a.lastToolID = id
	}
	if name, ok := block["name"].(string); ok && name != "" {
		a.commandName = name
		a.commandFound = true
	}
	if input, ok := block["input"].(map[string]interface{}); ok {
		payload, err := json.Marshal(input)
		if err == nil {
			if state := a.getOrCreateToolUse(index); state != nil {
				state.args = string(payload)
			}
		}
	}

	state := a.getOrCreateToolUse(index)
	if state != nil {
		state.id = a.lastToolID
		state.name = a.commandName
	}
}

func (a *guardrailsAccumulator) ingestContentBlockStop(index int) {
	if a.toolUses == nil {
		return
	}
	state, ok := a.toolUses[index]
	if !ok {
		return
	}
	a.completed = append(a.completed, *state)
	delete(a.toolUses, index)
}

func (a *guardrailsAccumulator) ingestOutputItem(item map[string]interface{}) {
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	if itemType != "function_call" && itemType != "custom_tool_call" && itemType != "mcp_call" {
		return
	}
	if id, ok := item["id"].(string); ok && id != "" {
		a.lastToolID = id
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

func (a *guardrailsAccumulator) captureIndex(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	if raw, ok := payload["index"]; ok {
		switch v := raw.(type) {
		case float64:
			a.lastIndex = int(v)
			a.hasIndex = true
			return a.lastIndex
		case int:
			a.lastIndex = v
			a.hasIndex = true
			return a.lastIndex
		case int64:
			a.lastIndex = int(v)
			a.hasIndex = true
			return a.lastIndex
		}
	}
	return 0
}

func (a *guardrailsAccumulator) nextBlockIndex() int {
	if a.hasIndex {
		return a.lastIndex + 1
	}
	return 0
}

func (a *guardrailsAccumulator) getOrCreateToolUse(index int) *toolUseState {
	if a.toolUses == nil {
		a.toolUses = make(map[int]*toolUseState)
	}
	if existing, ok := a.toolUses[index]; ok {
		return existing
	}
	state := &toolUseState{index: index}
	a.toolUses[index] = state
	return state
}

func (a *guardrailsAccumulator) popCompletedToolUse() (toolUseState, bool) {
	if len(a.completed) == 0 {
		return toolUseState{}, false
	}
	state := a.completed[0]
	a.completed = a.completed[1:]
	return state, true
}

// guardrailsBlockMessageWithSnippet formats a block message for text responses.
func guardrailsBlockMessageWithSnippet(result guardrails.Result, snippet string) string {
	prefix := "Blocked by guardrails. Content: text."
	suffix := ""
	if snippet != "" {
		suffix = " Snippet: \"" + snippet + "\""
	}
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return prefix + " Reason: " + result.Reasons[0].Reason + "." + suffix
	}
	if suffix != "" {
		return prefix + suffix
	}
	return prefix
}

// guardrailsBlockMessageForToolResult formats a block message for tool_result filtering.
func guardrailsBlockMessageForToolResult(result guardrails.Result) string {
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: tool_result. Output redacted. Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: tool_result. Output redacted."
}

// guardrailsBlockMessageForCommand formats a block message for tool_use command blocking.
func guardrailsBlockMessageForCommand(result guardrails.Result, name string, args map[string]interface{}) string {
	command := formatGuardrailsCommand(name, args)
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: command. Command: " + command + ". Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: command. Command: " + command + "."
}

// formatGuardrailsCommand renders a short command summary for block messages.
func formatGuardrailsCommand(name string, args map[string]interface{}) string {
	if name == "" {
		return "<unknown>"
	}
	if len(args) == 0 {
		return name + " {}"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return name + " {\"error\":\"marshal\"}"
	}
	const maxLen = 300
	payload := string(raw)
	if len(payload) > maxLen {
		payload = payload[:maxLen] + "..."
	}
	return name + " " + payload
}

func parseToolArgs(raw string) map[string]interface{} {
	if raw == "" {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return map[string]interface{}{"_raw": raw}
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
