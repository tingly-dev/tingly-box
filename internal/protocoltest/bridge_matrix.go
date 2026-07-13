package protocoltest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

const bridgeMatrixModel = "bridge-client-model"

// BridgeMatrix validates the dormant protocol Stage/Bridge topology without
// claiming to traverse the production gateway dispatch path. It intentionally
// uses the same TestResult shape as Matrix so CLI filtering and output remain
// consistent while result names make the execution surface explicit.
type BridgeMatrix struct {
	Pairs      []ProtocolPair
	Scenarios  []Scenario
	Streaming  []bool
	BatchCount int
}

// DefaultBridgePairs lists only concrete Stage boundaries implemented today.
// Anthropic v1/beta cross-version conversion remains intentionally absent.
func DefaultBridgePairs() []ProtocolPair {
	return []ProtocolPair{
		{Source: protocol.TypeAnthropicV1, Target: protocol.TypeAnthropicV1},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeAnthropicBeta},
		{Source: protocol.TypeAnthropicV1, Target: protocol.TypeOpenAIChat},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeOpenAIChat},
	}
}

// DefaultBridgeMatrix covers request/response semantics that the first
// Anthropic Bridges declare: text, tool use, tool results, complete, and stream.
func DefaultBridgeMatrix() *BridgeMatrix {
	return &BridgeMatrix{
		Pairs: DefaultBridgePairs(),
		Scenarios: []Scenario{
			TextScenario(),
			ToolUseScenario(),
			ToolResultScenario(),
		},
		Streaming: []bool{false, true},
	}
}

func (m *BridgeMatrix) clone() *BridgeMatrix {
	copy := *m
	return &copy
}

func (m *BridgeMatrix) OnlyScenarios(names ...string) *BridgeMatrix {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	filtered := make([]Scenario, 0, len(names))
	for _, scenario := range m.Scenarios {
		if wanted[scenario.Name] {
			filtered = append(filtered, scenario)
		}
	}
	out := m.clone()
	out.Scenarios = filtered
	return out
}

func (m *BridgeMatrix) OnlySources(sources ...string) *BridgeMatrix {
	wanted := make(map[protocol.APIType]bool, len(sources))
	for _, source := range sources {
		wanted[protocol.APIType(source)] = true
	}
	filtered := make([]ProtocolPair, 0, len(m.Pairs))
	for _, pair := range m.Pairs {
		if wanted[pair.Source] {
			filtered = append(filtered, pair)
		}
	}
	out := m.clone()
	out.Pairs = filtered
	return out
}

func (m *BridgeMatrix) OnlyTargets(targets ...string) *BridgeMatrix {
	wanted := make(map[protocol.APIType]bool, len(targets))
	for _, target := range targets {
		wanted[protocol.APIType(target)] = true
	}
	filtered := make([]ProtocolPair, 0, len(m.Pairs))
	for _, pair := range m.Pairs {
		if wanted[pair.Target] {
			filtered = append(filtered, pair)
		}
	}
	out := m.clone()
	out.Pairs = filtered
	return out
}

func (m *BridgeMatrix) OnlyStreaming(streaming bool) *BridgeMatrix {
	out := m.clone()
	out.Streaming = []bool{streaming}
	return out
}

func (m *BridgeMatrix) WithBatchCount(count int) *BridgeMatrix {
	out := m.clone()
	out.BatchCount = count
	return out
}

// ExecuteAll runs the in-process Bridge matrix and returns CLI-shaped results.
func (m *BridgeMatrix) ExecuteAll() []TestResult {
	results := make([]TestResult, 0, len(m.Pairs)*len(m.Scenarios)*len(m.Streaming))
	for _, scenario := range m.Scenarios {
		for _, pair := range m.Pairs {
			for _, streaming := range m.Streaming {
				if m.BatchCount > 1 {
					results = append(results, m.executeBatch(scenario, pair, streaming))
					continue
				}
				results = append(results, m.executeOne(scenario, pair, streaming))
			}
		}
	}
	return results
}

func (m *BridgeMatrix) executeOne(scenario Scenario, pair ProtocolPair, streaming bool) TestResult {
	start := time.Now()
	result := TestResult{
		Name:      fmt.Sprintf("bridges/%s/%s/%s/%s", scenario.Name, pair.Source, pair.Target, streamMode(streaming)),
		Scenario:  "bridges/" + scenario.Name,
		Source:    pair.Source,
		Target:    pair.Target,
		Streaming: streaming,
	}

	request, err := bridgeMatrixRequest(pair.Source, scenario.Name)
	if err != nil {
		return bridgeMatrixFailure(result, start, "fixture/request", err, "")
	}
	terminal, err := newBridgeMatrixTerminal(pair.Target, scenario.Name, streaming)
	if err != nil {
		return bridgeMatrixFailure(result, start, "fixture/terminal", err, "")
	}
	endpoint, err := bridgeMatrixEndpoint(terminal, pair)
	if err != nil {
		return bridgeMatrixFailure(result, start, "topology", err, "")
	}

	call := stage.Call{
		Request: request,
		Metadata: stage.CallMetadata{
			RequestID: fmt.Sprintf("bridge-matrix-%s-%s-%s", scenario.Name, pair.Source, streamMode(streaming)),
		},
	}
	var semantic *RoundTripResult
	var failures []AssertionError
	if streaming {
		stream, streamErr := endpoint.Stream(context.Background(), call)
		if streamErr != nil {
			return bridgeMatrixFailure(result, start, "stream/open", streamErr, "")
		}
		semantic, failures = consumeBridgeMatrixStream(stream, pair, scenario.Name)
		if terminal.lastStream == nil || terminal.lastStream.closeCount != 1 {
			failures = append(failures, AssertionError{
				Assertion: "stream/ownership",
				Error:     fmt.Sprintf("target close count = %d, want 1", terminal.streamCloseCount()),
			})
		}
	} else {
		response, completeErr := endpoint.Complete(context.Background(), call)
		if completeErr != nil {
			return bridgeMatrixFailure(result, start, "complete", completeErr, "")
		}
		semantic, failures = parseBridgeMatrixResponse(response, pair, scenario.Name)
	}

	failures = append(failures, validateBridgeMatrixTargetCall(terminal.lastCall, request, pair, scenario.Name, streaming)...)
	if semantic != nil {
		failures = append(failures, runBridgeMatrixAssertions(semantic, scenario.Name)...)
		result.Response = semantic
	}
	result.Passed = len(failures) == 0
	result.Errors = failures
	result.Duration = time.Since(start)
	return result
}

func (m *BridgeMatrix) executeBatch(scenario Scenario, pair ProtocolPair, streaming bool) TestResult {
	count := m.BatchCount
	runs := make([]TestResult, 0, count)
	passed := 0
	var total, min, max time.Duration
	uniqueErrors := make(map[string]AssertionError)
	for i := 0; i < count; i++ {
		run := m.executeOne(scenario, pair, streaming)
		runs = append(runs, run)
		total += run.Duration
		if i == 0 || run.Duration < min {
			min = run.Duration
		}
		if run.Duration > max {
			max = run.Duration
		}
		if run.Passed {
			passed++
		}
		for _, failure := range run.Errors {
			uniqueErrors[failure.Assertion+"\x00"+failure.Error] = failure
		}
	}
	errors := make([]AssertionError, 0, len(uniqueErrors))
	batchErrors := make([]string, 0, len(uniqueErrors))
	for _, failure := range uniqueErrors {
		errors = append(errors, failure)
		batchErrors = append(batchErrors, failure.Error)
	}
	last := runs[len(runs)-1]
	last.Passed = passed == count
	last.Errors = errors
	last.Duration = total / time.Duration(count)
	last.BatchCount = count
	last.BatchPassed = passed
	last.BatchMinDur = min
	last.BatchAvgDur = last.Duration
	last.BatchMaxDur = max
	last.BatchErrors = batchErrors
	return last
}

func bridgeMatrixFailure(result TestResult, start time.Time, assertion string, err error, context string) TestResult {
	result.Duration = time.Since(start)
	result.Errors = []AssertionError{{Assertion: assertion, Error: err.Error(), Context: context}}
	return result
}

func bridgeMatrixEndpoint(terminal stage.Endpoint, pair ProtocolPair) (stage.Endpoint, error) {
	registry, err := stage.NewBridgeRegistry(
		anthropicbridge.NewV1ToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
	)
	if err != nil {
		return nil, err
	}
	if pair.Source == pair.Target {
		identity, err := registry.Resolve(pair.Source, pair.Target, stage.AllBridgeCapabilities)
		if err != nil {
			return nil, err
		}
		return stage.Adapt(terminal, identity)
	}
	return stage.BuildTopology(stage.TopologyConfig{
		Terminal:             terminal,
		ClientProtocol:       pair.Source,
		Registry:             registry,
		RequiredCapabilities: stage.AllBridgeCapabilities,
	})
}

func bridgeMatrixRequest(source protocol.APIType, scenario string) (any, error) {
	messages := []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "What is the weather in Paris?"}}},
	}
	request := map[string]any{
		"model":      bridgeMatrixModel,
		"max_tokens": 128,
		"messages":   messages,
	}
	switch scenario {
	case "text":
	case "tool_use":
		request["tools"] = []any{bridgeMatrixToolDefinition()}
	case "tool_result":
		request["tools"] = []any{bridgeMatrixToolDefinition()}
		request["messages"] = []any{
			messages[0],
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "toolu_bridge_weather", "name": "get_weather",
				"input": map[string]any{"location": "Paris"},
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "toolu_bridge_weather", "content": "18°C and sunny", "is_error": false,
			}}},
		}
	default:
		return nil, fmt.Errorf("unsupported bridge scenario %q", scenario)
	}

	switch source {
	case protocol.TypeAnthropicV1:
		value, err := decodeBridgeFixture[anthropic.MessageNewParams](request)
		return &value, err
	case protocol.TypeAnthropicBeta:
		value, err := decodeBridgeFixture[anthropic.BetaMessageNewParams](request)
		return &value, err
	default:
		return nil, fmt.Errorf("unsupported bridge source %q", source)
	}
}

func bridgeMatrixToolDefinition() map[string]any {
	return map[string]any{
		"name":        "get_weather",
		"description": "Get weather for a location",
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"location": map[string]any{"type": "string"}},
			"required":   []string{"location"},
		},
	}
}

type bridgeMatrixTerminal struct {
	api        protocol.APIType
	response   *stage.Response
	events     []stage.Event
	result     stage.StreamResult
	lastCall   stage.Call
	lastStream *bridgeMatrixEventStream
}

func newBridgeMatrixTerminal(target protocol.APIType, scenario string, streaming bool) (*bridgeMatrixTerminal, error) {
	terminal := &bridgeMatrixTerminal{api: target}
	usage := protocol.NewTokenUsage(10, 8)
	terminal.result = stage.StreamResult{Usage: usage, Model: "bridge-provider-model"}
	if target == protocol.TypeOpenAIChat {
		if streaming {
			events, err := bridgeMatrixChatEvents(scenario)
			if err != nil {
				return nil, err
			}
			terminal.events = events
			return terminal, nil
		}
		response, err := bridgeMatrixChatResponse(scenario)
		if err != nil {
			return nil, err
		}
		terminal.response = &stage.Response{Value: response, Usage: usage, Model: "bridge-provider-model"}
		return terminal, nil
	}
	terminal.result.Model = bridgeMatrixModel

	if streaming {
		events, err := bridgeMatrixAnthropicEvents(target, scenario)
		if err != nil {
			return nil, err
		}
		terminal.events = events
		return terminal, nil
	}
	response, err := bridgeMatrixAnthropicResponse(target, scenario)
	if err != nil {
		return nil, err
	}
	terminal.response = &stage.Response{Value: response, Usage: usage, Model: bridgeMatrixModel}
	return terminal, nil
}

func (e *bridgeMatrixTerminal) Protocol() protocol.APIType { return e.api }

func (e *bridgeMatrixTerminal) Complete(_ context.Context, call stage.Call) (*stage.Response, error) {
	e.lastCall = call
	if e.response == nil {
		return nil, fmt.Errorf("bridge matrix terminal %q has no complete fixture", e.api)
	}
	copy := *e.response
	return &copy, nil
}

func (e *bridgeMatrixTerminal) Stream(_ context.Context, call stage.Call) (stage.EventStream, error) {
	e.lastCall = call
	e.lastStream = &bridgeMatrixEventStream{
		events: append([]stage.Event(nil), e.events...),
		result: e.result,
	}
	return e.lastStream, nil
}

func (e *bridgeMatrixTerminal) streamCloseCount() int {
	if e.lastStream == nil {
		return 0
	}
	return e.lastStream.closeCount
}

type bridgeMatrixEventStream struct {
	events     []stage.Event
	result     stage.StreamResult
	closeCount int
}

func (s *bridgeMatrixEventStream) Next(ctx context.Context) (stage.Event, error) {
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if len(s.events) == 0 {
		return stage.Event{}, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *bridgeMatrixEventStream) Close() error {
	s.closeCount++
	return nil
}

func (s *bridgeMatrixEventStream) Result() stage.StreamResult { return s.result }

func bridgeMatrixAnthropicResponse(target protocol.APIType, scenario string) (any, error) {
	body := bridgeMatrixAnthropicResponseBody(scenario)
	switch target {
	case protocol.TypeAnthropicV1:
		value, err := decodeBridgeFixture[anthropic.Message](body)
		return &value, err
	case protocol.TypeAnthropicBeta:
		value, err := decodeBridgeFixture[anthropic.BetaMessage](body)
		return &value, err
	default:
		return nil, fmt.Errorf("unsupported Anthropic target %q", target)
	}
}

func bridgeMatrixAnthropicResponseBody(scenario string) map[string]any {
	content := []any{map[string]any{"type": "text", "text": "The capital of France is Paris."}}
	stopReason := "end_turn"
	usage := map[string]any{"input_tokens": 10, "output_tokens": 8}
	if scenario == "tool_use" {
		content = []any{map[string]any{
			"type": "tool_use", "id": "toolu_bridge_weather", "name": "get_weather",
			"input": map[string]any{"location": "Paris", "unit": "celsius"},
		}}
		stopReason = "tool_use"
		usage = map[string]any{"input_tokens": 15, "output_tokens": 20}
	}
	return map[string]any{
		"id": "msg_bridge_matrix", "type": "message", "role": "assistant",
		"content": content, "model": bridgeMatrixModel, "stop_reason": stopReason,
		"stop_sequence": nil, "usage": usage,
	}
}

func bridgeMatrixChatResponse(scenario string) (*openai.ChatCompletion, error) {
	message := map[string]any{"role": "assistant", "content": "The capital of France is Paris."}
	finishReason := "stop"
	usage := map[string]any{"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
	if scenario == "tool_use" {
		message = map[string]any{
			"role": "assistant", "content": nil,
			"tool_calls": []any{map[string]any{
				"id": "call_bridge_weather", "type": "function",
				"function": map[string]any{"name": "get_weather", "arguments": `{"location":"Paris","unit":"celsius"}`},
			}},
		}
		finishReason = "tool_calls"
		usage = map[string]any{"prompt_tokens": 15, "completion_tokens": 20, "total_tokens": 35}
	}
	body := map[string]any{
		"id": "chatcmpl_bridge_matrix", "object": "chat.completion", "created": 1,
		"model":   "bridge-provider-model",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}},
		"usage":   usage,
	}
	value, err := decodeBridgeFixture[openai.ChatCompletion](body)
	return &value, err
}

func bridgeMatrixChatEvents(scenario string) ([]stage.Event, error) {
	var chunks []map[string]any
	if scenario == "tool_use" {
		chunks = []map[string]any{
			{
				"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": "bridge-provider-model",
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "", "delta": map[string]any{
						"role": "assistant", "tool_calls": []any{map[string]any{
							"index": 0, "id": "call_bridge_weather", "type": "function",
							"function": map[string]any{"name": "get_weather", "arguments": `{"location":"Paris","unit":"celsius"}`},
						}},
					},
				}},
			},
			bridgeMatrixChatFinishChunk("tool_calls"),
			bridgeMatrixChatUsageChunk(15, 20),
		}
	} else {
		chunks = []map[string]any{
			{
				"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": "bridge-provider-model",
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "", "delta": map[string]any{"role": "assistant", "content": "The capital of France is Paris."},
				}},
			},
			bridgeMatrixChatFinishChunk("stop"),
			bridgeMatrixChatUsageChunk(10, 8),
		}
	}
	events := make([]stage.Event, 0, len(chunks))
	for _, chunk := range chunks {
		value, err := decodeBridgeFixture[openai.ChatCompletionChunk](chunk)
		if err != nil {
			return nil, err
		}
		events = append(events, stage.Event{Value: value})
	}
	return events, nil
}

func bridgeMatrixChatFinishChunk(reason string) map[string]any {
	return map[string]any{
		"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": "bridge-provider-model",
		"choices": []any{map[string]any{"index": 0, "finish_reason": reason, "delta": map[string]any{}}},
	}
}

func bridgeMatrixChatUsageChunk(input, output int) map[string]any {
	return map[string]any{
		"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": "bridge-provider-model",
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output},
	}
}

func bridgeMatrixAnthropicEvents(target protocol.APIType, scenario string) ([]stage.Event, error) {
	inputTokens, outputTokens := 10, 8
	stopReason := "end_turn"
	var contentEvents []map[string]any
	if scenario == "tool_use" {
		inputTokens, outputTokens = 15, 20
		stopReason = "tool_use"
		contentEvents = []map[string]any{
			{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "toolu_bridge_weather", "name": "get_weather", "input": map[string]any{}}},
			{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"location":"Paris","unit":"celsius"}`}},
			{"type": "content_block_stop", "index": 0},
		}
	} else {
		contentEvents = []map[string]any{
			{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}},
			{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "The capital of France is Paris."}},
			{"type": "content_block_stop", "index": 0},
		}
	}
	events := []map[string]any{
		{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_bridge_stream", "type": "message", "role": "assistant", "content": []any{},
				"model": bridgeMatrixModel, "stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
			},
		},
	}
	events = append(events, contentEvents...)
	events = append(events,
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": outputTokens}},
		map[string]any{"type": "message_stop"},
	)

	result := make([]stage.Event, 0, len(events))
	for _, event := range events {
		switch target {
		case protocol.TypeAnthropicV1:
			value, err := decodeBridgeFixture[anthropic.MessageStreamEventUnion](event)
			if err != nil {
				return nil, err
			}
			result = append(result, stage.Event{Value: value})
		case protocol.TypeAnthropicBeta:
			value, err := decodeBridgeFixture[anthropic.BetaRawMessageStreamEventUnion](event)
			if err != nil {
				return nil, err
			}
			result = append(result, stage.Event{Value: value})
		default:
			return nil, fmt.Errorf("unsupported Anthropic target %q", target)
		}
	}
	return result, nil
}

func parseBridgeMatrixResponse(response *stage.Response, pair ProtocolPair, scenario string) (*RoundTripResult, []AssertionError) {
	if response == nil {
		return nil, []AssertionError{{Assertion: "response", Error: "response is nil"}}
	}
	var failures []AssertionError
	switch pair.Source {
	case protocol.TypeAnthropicV1:
		if _, ok := response.Value.(*anthropic.Message); !ok {
			failures = append(failures, AssertionError{Assertion: "response/type", Error: fmt.Sprintf("got %T, want *anthropic.Message", response.Value)})
		}
	case protocol.TypeAnthropicBeta:
		if _, ok := response.Value.(*anthropic.BetaMessage); !ok {
			failures = append(failures, AssertionError{Assertion: "response/type", Error: fmt.Sprintf("got %T, want *anthropic.BetaMessage", response.Value)})
		}
	}
	raw, err := json.Marshal(response.Value)
	if err != nil {
		failures = append(failures, AssertionError{Assertion: "response/marshal", Error: err.Error()})
		return nil, failures
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		failures = append(failures, AssertionError{Assertion: "response/decode", Error: err.Error(), Context: string(raw)})
		return nil, failures
	}
	parsed := sse.ParseAnthropicResult(body)
	result := roundTripFromBridgeParsed(parsed, pair, scenario, false, raw, nil)
	if response.Model != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "response/model_fact", Error: fmt.Sprintf("got %q, want %q", response.Model, bridgeMatrixModel)})
	}
	if response.Usage == nil || !response.Usage.HasUsage() {
		failures = append(failures, AssertionError{Assertion: "response/usage_fact", Error: fmt.Sprintf("missing normalized usage: %+v", response.Usage)})
	}
	return result, failures
}

func consumeBridgeMatrixStream(stream stage.EventStream, pair ProtocolPair, scenario string) (*RoundTripResult, []AssertionError) {
	var failures []AssertionError
	var eventLines []string
	for {
		event, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failures = append(failures, AssertionError{Assertion: "stream/next", Error: err.Error()})
			break
		}
		eventType, data, err := normalizeBridgeMatrixEvent(event.Value)
		if err != nil {
			failures = append(failures, AssertionError{Assertion: "stream/event", Error: err.Error()})
			continue
		}
		eventLines = append(eventLines, "event: "+eventType, "data: "+string(data))
	}
	streamResult := stream.Result()
	if streamResult.Model != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "stream/model_fact", Error: fmt.Sprintf("got %q, want %q", streamResult.Model, bridgeMatrixModel)})
	}
	if streamResult.Usage == nil || !streamResult.Usage.HasUsage() {
		failures = append(failures, AssertionError{Assertion: "stream/usage_fact", Error: fmt.Sprintf("missing normalized usage: %+v", streamResult.Usage)})
	}
	if err := stream.Close(); err != nil {
		failures = append(failures, AssertionError{Assertion: "stream/close", Error: err.Error()})
	}
	parsed := sse.AssembleAnthropicStream(eventLines)
	raw := []byte(strings.Join(eventLines, "\n") + "\n")
	return roundTripFromBridgeParsed(parsed, pair, scenario, true, raw, eventLines), failures
}

func normalizeBridgeMatrixEvent(value any) (string, []byte, error) {
	var eventType string
	var data any
	switch event := value.(type) {
	case protocolstream.AnthropicEvent:
		eventType, data = event.Type, event.Data
	case anthropic.MessageStreamEventUnion:
		eventType, data = event.Type, event
	case *anthropic.MessageStreamEventUnion:
		if event == nil {
			return "", nil, fmt.Errorf("nil Anthropic v1 event")
		}
		eventType, data = event.Type, event
	case anthropic.BetaRawMessageStreamEventUnion:
		eventType, data = event.Type, event
	case *anthropic.BetaRawMessageStreamEventUnion:
		if event == nil {
			return "", nil, fmt.Errorf("nil Anthropic beta event")
		}
		eventType, data = event.Type, event
	default:
		return "", nil, fmt.Errorf("unsupported Anthropic stream event %T", value)
	}
	if eventType == "" {
		return "", nil, fmt.Errorf("Anthropic stream event %T has empty type", value)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", nil, err
	}
	return eventType, raw, nil
}

func roundTripFromBridgeParsed(parsed *sse.ParsedResult, pair ProtocolPair, scenario string, streaming bool, raw []byte, events []string) *RoundTripResult {
	result := &RoundTripResult{
		SourceProtocol: pair.Source,
		TargetProtocol: pair.Target,
		ScenarioName:   "bridges/" + scenario,
		IsStreaming:    streaming,
		RawBody:        raw,
		StreamEvents:   events,
	}
	if parsed == nil {
		return result
	}
	result.Content = parsed.Content
	result.Role = parsed.Role
	result.Model = parsed.Model
	result.FinishReason = parsed.FinishReason
	result.ThinkingContent = parsed.ThinkingContent
	for _, toolCall := range parsed.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCallResult{
			ID: toolCall.ID, Name: toolCall.Name, Arguments: toolCall.Arguments,
		})
	}
	if parsed.Usage != nil {
		result.Usage = &TokenUsage{InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens}
	}
	return result
}

func runBridgeMatrixAssertions(result *RoundTripResult, scenario string) []AssertionError {
	assertions := []Assertion{AssertUsageNonZero()}
	switch scenario {
	case "text", "tool_result":
		assertions = append(assertions,
			AssertRoleEquals("assistant"),
			AssertContentContains("Paris"),
			AssertContentNonEmpty(),
		)
	case "tool_use":
		assertions = append(assertions,
			AssertHasToolCalls(1),
			AssertToolCallName(0, "get_weather"),
			AssertToolCallArgs(0, "location", "Paris"),
		)
	}
	var failures []AssertionError
	for _, assertion := range assertions {
		if err := assertion.Check(result); err != nil {
			failures = append(failures, AssertionError{
				Assertion: assertion.Name,
				Error:     err.Error(),
				Context:   truncate(string(result.RawBody), 300),
			})
		}
	}
	if result.Model != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "model/source_visible", Error: fmt.Sprintf("got %q, want %q", result.Model, bridgeMatrixModel)})
	}
	return failures
}

func validateBridgeMatrixTargetCall(call stage.Call, sourceRequest any, pair ProtocolPair, scenario string, streaming bool) []AssertionError {
	if call.Metadata.RequestID == "" {
		return []AssertionError{{Assertion: "request/metadata", Error: "request ID was not preserved"}}
	}
	if pair.Source == pair.Target {
		if call.Request != sourceRequest {
			return []AssertionError{{Assertion: "request/identity", Error: fmt.Sprintf("identity request changed from %T to %T", sourceRequest, call.Request)}}
		}
		return nil
	}

	chatRequest, ok := call.Request.(*openai.ChatCompletionNewParams)
	if !ok || chatRequest == nil {
		return []AssertionError{{Assertion: "request/type", Error: fmt.Sprintf("got %T, want *openai.ChatCompletionNewParams", call.Request)}}
	}
	var failures []AssertionError
	if string(chatRequest.Model) != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "request/model", Error: fmt.Sprintf("got %q, want %q", chatRequest.Model, bridgeMatrixModel)})
	}
	if call.State.OpenAIChat == nil {
		failures = append(failures, AssertionError{Assertion: "request/state", Error: "OpenAIConfig is nil"})
	}
	includeUsage := chatRequest.StreamOptions.IncludeUsage.Valid() && chatRequest.StreamOptions.IncludeUsage.Value
	if includeUsage != streaming {
		failures = append(failures, AssertionError{Assertion: "request/stream_usage", Error: fmt.Sprintf("include_usage=%v, want %v", includeUsage, streaming)})
	}
	raw, err := json.Marshal(chatRequest)
	if err != nil {
		failures = append(failures, AssertionError{Assertion: "request/marshal", Error: err.Error()})
		return failures
	}
	text := string(raw)
	if !strings.Contains(text, "Paris") {
		failures = append(failures, AssertionError{Assertion: "request/content", Error: "converted request lost Paris content", Context: truncate(text, 300)})
	}
	if (scenario == "tool_use" || scenario == "tool_result") && !strings.Contains(text, "get_weather") {
		failures = append(failures, AssertionError{Assertion: "request/tools", Error: "converted request lost get_weather", Context: truncate(text, 300)})
	}
	if scenario == "tool_result" {
		if !strings.Contains(text, `"role":"tool"`) || !strings.Contains(text, `"tool_call_id":"toolu_bridge_weather"`) {
			failures = append(failures, AssertionError{Assertion: "request/tool_result", Error: "converted request lost tool result linkage", Context: truncate(text, 300)})
		}
	}
	return failures
}

func decodeBridgeFixture[T any](value any) (T, error) {
	var result T
	raw, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
}
