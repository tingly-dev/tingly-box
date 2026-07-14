package toolloop

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

const defaultMaxRounds = 8

// OpenAIChatConfig constructs a Chat-native ToolLoop Stage. Catalog, policy,
// and executor are protocol-neutral dependencies; only this adapter understands
// OpenAI Chat request and response types.
type OpenAIChatConfig struct {
	Name      string
	Catalog   ToolCatalog
	Policy    ToolPolicy
	Executor  ToolExecutor
	MaxRounds int
}

// NewOpenAIChat returns a full-duplex Stage without attaching it to production
// routing. BuildTopology may later insert Bridges around this Chat-native level.
func NewOpenAIChat(config OpenAIChatConfig) (protocolstage.Stage, error) {
	if err := validateDependencies(config.Catalog, config.Executor); err != nil {
		return nil, fmt.Errorf("construct OpenAI Chat ToolLoop Stage: %w", err)
	}
	name := config.Name
	if name == "" {
		name = "tool_loop_openai_chat"
	}
	policy := config.Policy
	if policy == nil {
		policy = AllowAllPolicy{}
	}
	maxRounds := config.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	return &openAIChatStage{
		name:      name,
		catalog:   config.Catalog,
		policy:    policy,
		executor:  config.Executor,
		maxRounds: maxRounds,
	}, nil
}

type openAIChatStage struct {
	name      string
	catalog   ToolCatalog
	policy    ToolPolicy
	executor  ToolExecutor
	maxRounds int
}

func (s *openAIChatStage) Name() string             { return s.name }
func (*openAIChatStage) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }
func (s *openAIChatStage) Wrap(next protocolstage.Endpoint) protocolstage.Endpoint {
	return &openAIChatEndpoint{stage: s, next: next}
}

type openAIChatEndpoint struct {
	stage *openAIChatStage
	next  protocolstage.Endpoint
}

func (*openAIChatEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }

func (e *openAIChatEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	prepared, owned, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}

	runCtx := ctx
	current := prepared
	var totalUsage *protocol.TokenUsage
	sideEffectsCommitted := false
	for round := 1; round <= e.stage.maxRounds; round++ {
		response, callErr := e.next.Complete(runCtx, current)
		if callErr != nil {
			return nil, WrapError(callErr, sideEffectsCommitted)
		}
		if response == nil {
			return nil, WrapError(errors.New("OpenAI Chat ToolLoop received a nil response"), sideEffectsCommitted)
		}
		totalUsage = mergeTokenUsage(totalUsage, response.Usage)
		sideEffectsCommitted = sideEffectsCommitted || response.SideEffectsCommitted

		roundResponse, parseErr := parseChatRound(response.Value)
		if parseErr != nil {
			return nil, WrapError(parseErr, sideEffectsCommitted)
		}
		if !allCallsOwned(roundResponse.calls, owned) {
			response.Usage = totalUsage
			response.SideEffectsCommitted = sideEffectsCommitted
			return response, nil
		}
		if round == e.stage.maxRounds {
			return nil, WrapError(ErrMaxRounds, sideEffectsCommitted)
		}

		results, nextCtx, committed, executeErr := e.executeCalls(runCtx, roundResponse.calls)
		sideEffectsCommitted = sideEffectsCommitted || committed
		if executeErr != nil {
			return nil, WrapError(executeErr, sideEffectsCommitted)
		}
		runCtx = nextCtx
		nextRequest, appendErr := appendChatToolResults(current.Request, roundResponse.assistant, results)
		if appendErr != nil {
			return nil, WrapError(appendErr, sideEffectsCommitted)
		}
		current.Request = nextRequest
	}
	return nil, WrapError(ErrMaxRounds, sideEffectsCommitted)
}

func (e *openAIChatEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	prepared, owned, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	return newOpenAIChatToolLoopStream(ctx, e, prepared, owned)
}

func (e *openAIChatEndpoint) prepare(ctx context.Context, call protocolstage.Call) (protocolstage.Call, map[string]struct{}, error) {
	request, ok := call.Request.(*openai.ChatCompletionNewParams)
	if !ok || request == nil {
		return protocolstage.Call{}, nil, fmt.Errorf("OpenAI Chat ToolLoop received request %T", call.Request)
	}
	definitions, err := e.stage.catalog.ListTools(ctx)
	if err != nil {
		return protocolstage.Call{}, nil, fmt.Errorf("list ToolLoop catalog: %w", err)
	}
	if err := validateDefinitions(definitions); err != nil {
		return protocolstage.Call{}, nil, err
	}

	cloned := *request
	cloned.Messages = append([]openai.ChatCompletionMessageParamUnion(nil), request.Messages...)
	cloned.Tools = append([]openai.ChatCompletionToolUnionParam(nil), request.Tools...)
	existing := make(map[string]struct{}, len(cloned.Tools))
	for _, tool := range cloned.Tools {
		if function := tool.GetFunction(); function != nil && function.Name != "" {
			existing[function.Name] = struct{}{}
		}
	}
	owned := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		owned[definition.Name] = struct{}{}
		if _, exists := existing[definition.Name]; exists {
			continue
		}
		function := shared.FunctionDefinitionParam{
			Name:       definition.Name,
			Parameters: cloneParameters(definition.Parameters),
		}
		if definition.Description != "" {
			function.Description = param.NewOpt(definition.Description)
		}
		cloned.Tools = append(cloned.Tools, openai.ChatCompletionFunctionTool(function))
	}
	prepared := call
	prepared.Request = &cloned
	return prepared, owned, nil
}

func (e *openAIChatEndpoint) executeCalls(ctx context.Context, calls []ToolCall) ([]ToolResult, context.Context, bool, error) {
	for _, call := range calls {
		if err := e.stage.policy.Authorize(ctx, call); err != nil {
			return nil, ctx, false, fmt.Errorf("authorize tool %q: %w", call.Name, err)
		}
	}

	results := make([]ToolResult, 0, len(calls))
	runCtx := ctx
	committed := false
	for _, call := range calls {
		nextCtx, result, err := e.stage.executor.Execute(runCtx, call)
		if nextCtx != nil {
			runCtx = nextCtx
		}
		if result.ToolCallID == "" {
			result.ToolCallID = call.ID
		}
		if err != nil {
			result.IsError = true
			if result.Content == "" {
				result.Content = err.Error()
			}
		} else {
			committed = true
		}
		results = append(results, result)
	}
	return results, runCtx, committed, nil
}

type chatRound struct {
	assistant openai.ChatCompletionMessageParamUnion
	calls     []ToolCall
}

func parseChatRound(value any) (chatRound, error) {
	switch response := value.(type) {
	case openai.ChatCompletion:
		return parseSDKChatRound(&response)
	case *openai.ChatCompletion:
		return parseSDKChatRound(response)
	case wire.ChatCompletionWire:
		return parseWireChatRound(&response)
	case *wire.ChatCompletionWire:
		return parseWireChatRound(response)
	default:
		return chatRound{}, fmt.Errorf("OpenAI Chat ToolLoop received response %T", value)
	}
}

func parseSDKChatRound(response *openai.ChatCompletion) (chatRound, error) {
	if response == nil || len(response.Choices) == 0 {
		return chatRound{assistant: openai.AssistantMessage("")}, nil
	}
	message := response.Choices[0].Message
	calls := make([]ToolCall, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		if call.Type != "function" {
			continue
		}
		calls = append(calls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return chatRound{assistant: message.ToParam(), calls: calls}, nil
}

func parseWireChatRound(response *wire.ChatCompletionWire) (chatRound, error) {
	if response == nil || len(response.Choices) == 0 {
		return chatRound{assistant: openai.AssistantMessage("")}, nil
	}
	message := response.Choices[0].Message
	assistant := openai.AssistantMessage(message.Content)
	calls := make([]ToolCall, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			continue
		}
		calls = append(calls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
		assistant.OfAssistant.ToolCalls = append(assistant.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				},
			},
		})
	}
	return chatRound{assistant: assistant, calls: calls}, nil
}

func allCallsOwned(calls []ToolCall, owned map[string]struct{}) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		if _, ok := owned[call.Name]; !ok {
			return false
		}
	}
	return true
}

func appendChatToolResults(request any, assistant openai.ChatCompletionMessageParamUnion, results []ToolResult) (*openai.ChatCompletionNewParams, error) {
	params, ok := request.(*openai.ChatCompletionNewParams)
	if !ok || params == nil {
		return nil, fmt.Errorf("append ToolLoop results to request %T", request)
	}
	cloned := *params
	cloned.Messages = append([]openai.ChatCompletionMessageParamUnion(nil), params.Messages...)
	cloned.Messages = append(cloned.Messages, assistant)
	for _, result := range results {
		content := result.Content
		if result.IsError && content == "" {
			content = "tool execution failed"
		}
		cloned.Messages = append(cloned.Messages, openai.ToolMessage(content, result.ToolCallID))
	}
	return &cloned, nil
}

func mergeTokenUsage(total, current *protocol.TokenUsage) *protocol.TokenUsage {
	if current == nil {
		return total
	}
	if total == nil {
		copy := *current
		return &copy
	}
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	total.CacheInputTokens += current.CacheInputTokens
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	total.ReasoningTokens += current.ReasoningTokens
	total.SystemTokens += current.SystemTokens
	return total
}

func cloneParameters(parameters map[string]any) shared.FunctionParameters {
	if parameters == nil {
		return nil
	}
	cloned := make(shared.FunctionParameters, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
}
