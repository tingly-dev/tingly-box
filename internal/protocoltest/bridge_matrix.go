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
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

const bridgeMatrixModel = "bridge-client-model"

// BridgeMatrix validates the dormant protocol Stage/Bridge topology without
// claiming to traverse the production gateway dispatch path. It intentionally
// uses the same TestResult shape as Matrix so CLI filtering and output remain
// consistent while result names make the execution surface explicit.
type BridgeMatrix struct {
	Pairs      []ProtocolPair
	Chains     []BridgeChain
	Scenarios  []Scenario
	Streaming  []bool
	BatchCount int
}

// BridgeChain describes one concrete multi-level topology. Source and Target
// are the client and terminal protocols; Stage is the native middle protocol.
type BridgeChain struct {
	Name   string
	Source protocol.APIType
	Stage  protocol.APIType
	Target protocol.APIType
}

// DefaultBridgePairs lists only concrete Stage boundaries implemented today.
// Anthropic v1/beta cross-version conversion remains intentionally absent.
func DefaultBridgePairs() []ProtocolPair {
	return []ProtocolPair{
		{Source: protocol.TypeAnthropicV1, Target: protocol.TypeAnthropicV1},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeAnthropicBeta},
		{Source: protocol.TypeOpenAIChat, Target: protocol.TypeOpenAIChat},
		{Source: protocol.TypeAnthropicV1, Target: protocol.TypeOpenAIChat},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeOpenAIChat},
		{Source: protocol.TypeOpenAIChat, Target: protocol.TypeAnthropicBeta},
	}
}

// DefaultBridgeChains contains real concrete Bridges on both sides of an
// Anthropic Beta-native Stage. It remains in-process and carries no production
// traffic.
func DefaultBridgeChains() []BridgeChain {
	return []BridgeChain{
		{
			Name:   "chat_beta_stage_chat",
			Source: protocol.TypeOpenAIChat,
			Stage:  protocol.TypeAnthropicBeta,
			Target: protocol.TypeOpenAIChat,
		},
	}
}

// DefaultBridgeMatrix covers request/response semantics that the first
// Anthropic Bridges declare: text, tool use, tool results, complete, and stream.
func DefaultBridgeMatrix() *BridgeMatrix {
	return &BridgeMatrix{
		Pairs:  DefaultBridgePairs(),
		Chains: DefaultBridgeChains(),
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
	filteredChains := make([]BridgeChain, 0, len(m.Chains))
	for _, chain := range m.Chains {
		if wanted[chain.Source] {
			filteredChains = append(filteredChains, chain)
		}
	}
	out.Chains = filteredChains
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
	filteredChains := make([]BridgeChain, 0, len(m.Chains))
	for _, chain := range m.Chains {
		if wanted[chain.Target] {
			filteredChains = append(filteredChains, chain)
		}
	}
	out.Chains = filteredChains
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
	routes := make([]bridgeMatrixRoute, 0, len(m.Pairs)+len(m.Chains))
	for _, pair := range m.Pairs {
		routes = append(routes, bridgeMatrixRoute{Pair: pair})
	}
	for _, chain := range m.Chains {
		routes = append(routes, bridgeMatrixRoute{
			Name:  chain.Name,
			Pair:  ProtocolPair{Source: chain.Source, Target: chain.Target},
			Stage: chain.Stage,
		})
	}
	results := make([]TestResult, 0, len(routes)*len(m.Scenarios)*len(m.Streaming))
	for _, scenario := range m.Scenarios {
		for _, route := range routes {
			for _, streaming := range m.Streaming {
				if m.BatchCount > 1 {
					results = append(results, m.executeBatch(scenario, route, streaming))
					continue
				}
				results = append(results, m.executeOne(scenario, route, streaming))
			}
		}
	}
	return results
}

type bridgeMatrixRoute struct {
	Name  string
	Pair  ProtocolPair
	Stage protocol.APIType
}

func (r bridgeMatrixRoute) isChain() bool { return r.Name != "" }

func (r bridgeMatrixRoute) scenarioName(scenario string) string {
	if r.isChain() {
		return "bridges/chain/" + r.Name + "/" + scenario
	}
	return "bridges/" + scenario
}

func (r bridgeMatrixRoute) resultName(scenario string, streaming bool) string {
	if r.isChain() {
		return fmt.Sprintf("bridges/chain/%s/%s/%s/%s/%s", r.Name, scenario, r.Pair.Source, r.Pair.Target, streamMode(streaming))
	}
	return fmt.Sprintf("bridges/%s/%s/%s/%s", scenario, r.Pair.Source, r.Pair.Target, streamMode(streaming))
}

func (m *BridgeMatrix) executeOne(scenario Scenario, route bridgeMatrixRoute, streaming bool) TestResult {
	start := time.Now()
	pair := route.Pair
	result := TestResult{
		Name:      route.resultName(scenario.Name, streaming),
		Scenario:  route.scenarioName(scenario.Name),
		Source:    pair.Source,
		Target:    pair.Target,
		Streaming: streaming,
	}

	request, err := bridgeMatrixRequest(pair.Source, scenario.Name)
	if err != nil {
		return bridgeMatrixFailure(result, start, "fixture/request", err, "")
	}
	terminal, err := newBridgeMatrixTerminal(route, scenario.Name, streaming)
	if err != nil {
		return bridgeMatrixFailure(result, start, "fixture/terminal", err, "")
	}
	endpoint, probe, err := bridgeMatrixEndpoint(terminal, route)
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
		semantic, failures = consumeBridgeMatrixStream(stream, route, scenario.Name)
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
		semantic, failures = parseBridgeMatrixResponse(response, route, scenario.Name)
	}

	failures = append(failures, validateBridgeMatrixTargetCall(terminal.lastCall, request, route, scenario.Name, streaming)...)
	failures = append(failures, validateBridgeMatrixProbe(probe, streaming)...)
	if semantic != nil {
		failures = append(failures, runBridgeMatrixAssertions(semantic, scenario.Name)...)
		result.Response = semantic
	}
	result.Passed = len(failures) == 0
	result.Errors = failures
	result.Duration = time.Since(start)
	return result
}

func (m *BridgeMatrix) executeBatch(scenario Scenario, route bridgeMatrixRoute, streaming bool) TestResult {
	count := m.BatchCount
	runs := make([]TestResult, 0, count)
	passed := 0
	var total, min, max time.Duration
	uniqueErrors := make(map[string]AssertionError)
	for i := 0; i < count; i++ {
		run := m.executeOne(scenario, route, streaming)
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

func bridgeMatrixEndpoint(terminal stage.Endpoint, route bridgeMatrixRoute) (stage.Endpoint, *bridgeMatrixProbeStage, error) {
	registry, err := stage.NewBridgeRegistry(
		anthropicbridge.NewV1ToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
		openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{}),
	)
	if err != nil {
		return nil, nil, err
	}
	if !route.isChain() && route.Pair.Source == route.Pair.Target {
		identity, err := registry.Resolve(route.Pair.Source, route.Pair.Target, stage.AllBridgeCapabilities)
		if err != nil {
			return nil, nil, err
		}
		endpoint, err := stage.Adapt(terminal, identity)
		return endpoint, nil, err
	}
	var (
		stages []stage.Stage
		probe  *bridgeMatrixProbeStage
	)
	if route.isChain() {
		probe = &bridgeMatrixProbeStage{api: route.Stage}
		stages = []stage.Stage{probe}
	}
	endpoint, err := stage.BuildTopology(stage.TopologyConfig{
		Terminal:             terminal,
		Stages:               stages,
		ClientProtocol:       route.Pair.Source,
		Registry:             registry,
		RequiredCapabilities: stage.AllBridgeCapabilities,
	})
	return endpoint, probe, err
}

// bridgeMatrixProbeStage proves that the concrete middle level receives and
// returns its declared native protocol in both execution modes.
type bridgeMatrixProbeStage struct {
	api protocol.APIType

	completeRequest  any
	completeResponse any
	streamRequest    any
	streamEvents     int
	streamTypeError  error
}

func (*bridgeMatrixProbeStage) Name() string { return "bridge-matrix-probe" }

func (s *bridgeMatrixProbeStage) Protocol() protocol.APIType { return s.api }

func (s *bridgeMatrixProbeStage) Wrap(next stage.Endpoint) stage.Endpoint {
	return &bridgeMatrixProbeEndpoint{stage: s, next: next}
}

type bridgeMatrixProbeEndpoint struct {
	stage *bridgeMatrixProbeStage
	next  stage.Endpoint
}

func (e *bridgeMatrixProbeEndpoint) Protocol() protocol.APIType { return e.stage.api }

func (e *bridgeMatrixProbeEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	e.stage.completeRequest = call.Request
	response, err := e.next.Complete(ctx, call)
	if response != nil {
		e.stage.completeResponse = response.Value
	}
	return response, err
}

func (e *bridgeMatrixProbeEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	e.stage.streamRequest = call.Request
	stream, err := e.next.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	return &bridgeMatrixProbeStream{stage: e.stage, next: stream}, nil
}

type bridgeMatrixProbeStream struct {
	stage *bridgeMatrixProbeStage
	next  stage.EventStream
}

func (s *bridgeMatrixProbeStream) Next(ctx context.Context) (stage.Event, error) {
	event, err := s.next.Next(ctx)
	if err != nil {
		return event, err
	}
	s.stage.streamEvents++
	switch event.Value.(type) {
	case protocolstream.AnthropicEvent, anthropic.BetaRawMessageStreamEventUnion, *anthropic.BetaRawMessageStreamEventUnion:
	default:
		s.stage.streamTypeError = fmt.Errorf("middle Stage received %T, want Anthropic Beta event", event.Value)
	}
	return event, nil
}

func (s *bridgeMatrixProbeStream) Close() error { return s.next.Close() }

func (s *bridgeMatrixProbeStream) Result() stage.StreamResult { return s.next.Result() }

func validateBridgeMatrixProbe(probe *bridgeMatrixProbeStage, streaming bool) []AssertionError {
	if probe == nil {
		return nil
	}
	var failures []AssertionError
	if streaming {
		if _, ok := probe.streamRequest.(*anthropic.BetaMessageNewParams); !ok {
			failures = append(failures, AssertionError{Assertion: "chain/stage_request", Error: fmt.Sprintf("got %T, want *anthropic.BetaMessageNewParams", probe.streamRequest)})
		}
		if probe.streamEvents == 0 {
			failures = append(failures, AssertionError{Assertion: "chain/stage_events", Error: "middle Stage observed no response events"})
		}
		if probe.streamTypeError != nil {
			failures = append(failures, AssertionError{Assertion: "chain/stage_event_type", Error: probe.streamTypeError.Error()})
		}
		return failures
	}
	if _, ok := probe.completeRequest.(*anthropic.BetaMessageNewParams); !ok {
		failures = append(failures, AssertionError{Assertion: "chain/stage_request", Error: fmt.Sprintf("got %T, want *anthropic.BetaMessageNewParams", probe.completeRequest)})
	}
	if _, ok := probe.completeResponse.(*anthropic.BetaMessage); !ok {
		failures = append(failures, AssertionError{Assertion: "chain/stage_response", Error: fmt.Sprintf("got %T, want *anthropic.BetaMessage", probe.completeResponse)})
	}
	return failures
}

func bridgeMatrixRequest(source protocol.APIType, scenario string) (any, error) {
	if source == protocol.TypeOpenAIChat {
		return bridgeMatrixChatRequest(scenario)
	}
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

func bridgeMatrixChatRequest(scenario string) (any, error) {
	request := map[string]any{
		"model":      bridgeMatrixModel,
		"max_tokens": 128,
		"messages": []any{
			map[string]any{"role": "user", "content": "What is the weather in Paris?"},
		},
	}
	switch scenario {
	case "text":
	case "tool_use":
		request["tools"] = []any{bridgeMatrixChatToolDefinition()}
	case "tool_result":
		request["tools"] = []any{bridgeMatrixChatToolDefinition()}
		request["messages"] = []any{
			map[string]any{"role": "user", "content": "What is the weather in Paris?"},
			map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{
					"id": "toolu_bridge_weather", "type": "function",
					"function": map[string]any{"name": "get_weather", "arguments": `{"location":"Paris"}`},
				}},
			},
			map[string]any{"role": "tool", "tool_call_id": "toolu_bridge_weather", "content": "18°C and sunny"},
		}
	default:
		return nil, fmt.Errorf("unsupported bridge scenario %q", scenario)
	}
	value, err := decodeBridgeFixture[openai.ChatCompletionNewParams](request)
	return &value, err
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

func bridgeMatrixChatToolDefinition() map[string]any {
	definition := bridgeMatrixToolDefinition()
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        definition["name"],
			"description": definition["description"],
			"parameters":  definition["input_schema"],
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

func newBridgeMatrixTerminal(route bridgeMatrixRoute, scenario string, streaming bool) (*bridgeMatrixTerminal, error) {
	target := route.Pair.Target
	terminal := &bridgeMatrixTerminal{api: target}
	usage := protocol.NewTokenUsage(10, 8)
	terminalModel := "bridge-provider-model"
	if !route.isChain() && route.Pair.Source == route.Pair.Target {
		terminalModel = bridgeMatrixModel
	}
	terminal.result = stage.StreamResult{Usage: usage, Model: terminalModel}
	if target == protocol.TypeOpenAIChat {
		if streaming {
			events, err := bridgeMatrixChatEvents(scenario, terminalModel)
			if err != nil {
				return nil, err
			}
			terminal.events = events
			return terminal, nil
		}
		response, err := bridgeMatrixChatResponse(scenario, terminalModel)
		if err != nil {
			return nil, err
		}
		terminal.response = &stage.Response{Value: response, Usage: usage, Model: terminalModel}
		return terminal, nil
	}

	if streaming {
		events, err := bridgeMatrixAnthropicEvents(target, scenario, terminalModel)
		if err != nil {
			return nil, err
		}
		terminal.events = events
		return terminal, nil
	}
	response, err := bridgeMatrixAnthropicResponse(target, scenario, terminalModel)
	if err != nil {
		return nil, err
	}
	terminal.response = &stage.Response{Value: response, Usage: usage, Model: terminalModel}
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

func bridgeMatrixAnthropicResponse(target protocol.APIType, scenario, model string) (any, error) {
	body := bridgeMatrixAnthropicResponseBody(scenario, model)
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

func bridgeMatrixAnthropicResponseBody(scenario, model string) map[string]any {
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
		"content": content, "model": model, "stop_reason": stopReason,
		"stop_sequence": nil, "usage": usage,
	}
}

func bridgeMatrixChatResponse(scenario, model string) (*openai.ChatCompletion, error) {
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
		"model":   model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}},
		"usage":   usage,
	}
	value, err := decodeBridgeFixture[openai.ChatCompletion](body)
	return &value, err
}

func bridgeMatrixChatEvents(scenario, model string) ([]stage.Event, error) {
	var chunks []map[string]any
	if scenario == "tool_use" {
		chunks = []map[string]any{
			{
				"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": model,
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "", "delta": map[string]any{
						"role": "assistant", "tool_calls": []any{map[string]any{
							"index": 0, "id": "call_bridge_weather", "type": "function",
							"function": map[string]any{"name": "get_weather", "arguments": `{"location":"Paris","unit":"celsius"}`},
						}},
					},
				}},
			},
			bridgeMatrixChatFinishChunk("tool_calls", model),
			bridgeMatrixChatUsageChunk(15, 20, model),
		}
	} else {
		chunks = []map[string]any{
			{
				"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": model,
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "", "delta": map[string]any{"role": "assistant", "content": "The capital of France is Paris."},
				}},
			},
			bridgeMatrixChatFinishChunk("stop", model),
			bridgeMatrixChatUsageChunk(10, 8, model),
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

func bridgeMatrixChatFinishChunk(reason, model string) map[string]any {
	return map[string]any{
		"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": model,
		"choices": []any{map[string]any{"index": 0, "finish_reason": reason, "delta": map[string]any{}}},
	}
}

func bridgeMatrixChatUsageChunk(input, output int, model string) map[string]any {
	return map[string]any{
		"id": "chatcmpl_bridge_stream", "object": "chat.completion.chunk", "created": 1, "model": model,
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output},
	}
}

func bridgeMatrixAnthropicEvents(target protocol.APIType, scenario, model string) ([]stage.Event, error) {
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
				"model": model, "stop_reason": nil, "stop_sequence": nil,
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

func parseBridgeMatrixResponse(response *stage.Response, route bridgeMatrixRoute, scenario string) (*RoundTripResult, []AssertionError) {
	if response == nil {
		return nil, []AssertionError{{Assertion: "response", Error: "response is nil"}}
	}
	pair := route.Pair
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
	case protocol.TypeOpenAIChat:
		switch response.Value.(type) {
		case *openai.ChatCompletion, openai.ChatCompletion, wire.ChatCompletionWire, *wire.ChatCompletionWire:
		default:
			failures = append(failures, AssertionError{Assertion: "response/type", Error: fmt.Sprintf("got %T, want OpenAI Chat response value", response.Value)})
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
	var parsed *sse.ParsedResult
	if pair.Source == protocol.TypeOpenAIChat {
		parsed = sse.ParseOpenAIChatResult(body)
	} else {
		parsed = sse.ParseAnthropicResult(body)
	}
	result := roundTripFromBridgeParsed(parsed, route, scenario, false, raw, nil)
	if response.Model != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "response/model_fact", Error: fmt.Sprintf("got %q, want %q", response.Model, bridgeMatrixModel)})
	}
	if response.Usage == nil || !response.Usage.HasUsage() {
		failures = append(failures, AssertionError{Assertion: "response/usage_fact", Error: fmt.Sprintf("missing normalized usage: %+v", response.Usage)})
	}
	return result, failures
}

func consumeBridgeMatrixStream(stream stage.EventStream, route bridgeMatrixRoute, scenario string) (*RoundTripResult, []AssertionError) {
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
		if route.Pair.Source == protocol.TypeOpenAIChat {
			data, err := normalizeBridgeMatrixChatEvent(event.Value)
			if err != nil {
				failures = append(failures, AssertionError{Assertion: "stream/event", Error: err.Error()})
				continue
			}
			eventLines = append(eventLines, "data: "+string(data))
		} else {
			eventType, data, err := normalizeBridgeMatrixEvent(event.Value)
			if err != nil {
				failures = append(failures, AssertionError{Assertion: "stream/event", Error: err.Error()})
				continue
			}
			eventLines = append(eventLines, "event: "+eventType, "data: "+string(data))
		}
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
	var parsed *sse.ParsedResult
	if route.Pair.Source == protocol.TypeOpenAIChat {
		parsed = sse.AssembleOpenAIChatStream(eventLines)
	} else {
		parsed = sse.AssembleAnthropicStream(eventLines)
	}
	raw := []byte(strings.Join(eventLines, "\n") + "\n")
	result := roundTripFromBridgeParsed(parsed, route, scenario, true, raw, eventLines)
	if result.Usage == nil && streamResult.Usage != nil {
		result.Usage = &TokenUsage{InputTokens: streamResult.Usage.InputTokens, OutputTokens: streamResult.Usage.OutputTokens}
	}
	return result, failures
}

func normalizeBridgeMatrixChatEvent(value any) ([]byte, error) {
	switch event := value.(type) {
	case openai.ChatCompletionChunk:
		return json.Marshal(event)
	case *openai.ChatCompletionChunk:
		if event == nil {
			return nil, fmt.Errorf("nil OpenAI Chat chunk")
		}
		return json.Marshal(event)
	case wire.ChatStreamChunk:
		return json.Marshal(event)
	case *wire.ChatStreamChunk:
		if event == nil {
			return nil, fmt.Errorf("nil OpenAI Chat wire chunk")
		}
		return json.Marshal(event)
	default:
		return nil, fmt.Errorf("unsupported OpenAI Chat stream event %T", value)
	}
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

func roundTripFromBridgeParsed(parsed *sse.ParsedResult, route bridgeMatrixRoute, scenario string, streaming bool, raw []byte, events []string) *RoundTripResult {
	result := &RoundTripResult{
		SourceProtocol: route.Pair.Source,
		TargetProtocol: route.Pair.Target,
		ScenarioName:   route.scenarioName(scenario),
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

func validateBridgeMatrixTargetCall(call stage.Call, sourceRequest any, route bridgeMatrixRoute, scenario string, streaming bool) []AssertionError {
	if call.Metadata.RequestID == "" {
		return []AssertionError{{Assertion: "request/metadata", Error: "request ID was not preserved"}}
	}
	if !route.isChain() && route.Pair.Source == route.Pair.Target {
		if call.Request != sourceRequest {
			return []AssertionError{{Assertion: "request/identity", Error: fmt.Sprintf("identity request changed from %T to %T", sourceRequest, call.Request)}}
		}
		return nil
	}
	if route.Pair.Target == protocol.TypeAnthropicBeta {
		return validateBridgeMatrixBetaTargetCall(call, scenario)
	}
	return validateBridgeMatrixChatTargetCall(call, scenario, streaming)
}

func validateBridgeMatrixChatTargetCall(call stage.Call, scenario string, streaming bool) []AssertionError {
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

func validateBridgeMatrixBetaTargetCall(call stage.Call, scenario string) []AssertionError {
	request, ok := call.Request.(*anthropic.BetaMessageNewParams)
	if !ok || request == nil {
		return []AssertionError{{Assertion: "request/type", Error: fmt.Sprintf("got %T, want *anthropic.BetaMessageNewParams", call.Request)}}
	}
	var failures []AssertionError
	if string(request.Model) != bridgeMatrixModel {
		failures = append(failures, AssertionError{Assertion: "request/model", Error: fmt.Sprintf("got %q, want %q", request.Model, bridgeMatrixModel)})
	}
	if call.State.OpenAIChat != nil {
		failures = append(failures, AssertionError{Assertion: "request/state", Error: "OpenAIConfig leaked into Anthropic target"})
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return append(failures, AssertionError{Assertion: "request/marshal", Error: err.Error()})
	}
	text := string(raw)
	if !strings.Contains(text, "Paris") {
		failures = append(failures, AssertionError{Assertion: "request/content", Error: "converted request lost Paris content", Context: truncate(text, 300)})
	}
	if (scenario == "tool_use" || scenario == "tool_result") && !strings.Contains(text, "get_weather") {
		failures = append(failures, AssertionError{Assertion: "request/tools", Error: "converted request lost get_weather", Context: truncate(text, 300)})
	}
	if scenario == "tool_result" && (!strings.Contains(text, `"type":"tool_result"`) || !strings.Contains(text, `"tool_use_id":"toolu_bridge_weather"`)) {
		failures = append(failures, AssertionError{Assertion: "request/tool_result", Error: "converted request lost tool result linkage", Context: truncate(text, 300)})
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
