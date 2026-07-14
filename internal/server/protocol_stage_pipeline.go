package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

const protocolPipelineHeader = "X-Tingly-Protocol-Pipeline"

// tryProtocolStageOpenAIChat selects and executes the Stage path for one
// provider attempt. Returning false means the caller must continue through the
// legacy transform/dispatch path. Returning true means this method has fully
// handled the attempt, including any response or error.
func (ph *ProtocolHandler) tryProtocolStageOpenAIChat(
	c *gin.Context,
	req *protocol.OpenAIChatCompletionRequest,
	responseModel string,
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	rule *typ.Rule,
	isStreaming bool,
	scenarioConfig *typ.ScenarioConfig,
	ruleFlags typ.RuleFlags,
	stageRecording *protocolStageRequestRecording,
) bool {
	if !ph.shouldUseProtocolStage(
		c,
		protocol.TypeOpenAIChat,
		target,
		protocolstage.AllBridgeCapabilities,
	) {
		return false
	}

	// The generic MCP transform/tool loop has not moved behind a Stage yet.
	// Keeping the whole request on legacy is safer than silently omitting tool
	// injection or executing tools through two owners.
	if ph.mcpEnabled() {
		logProtocolStageFallback(c, protocol.TypeOpenAIChat, target, "MCP runtime still uses the legacy pipeline")
		return false
	}
	// The response-roundtrip header is an explicit legacy diagnostic. Preserve
	// that exact experiment instead of changing its semantics under --stage.
	if target == protocol.TypeAnthropicBeta && ShouldRoundtripResponse(c, "anthropic") {
		logProtocolStageFallback(c, protocol.TypeOpenAIChat, target, "response roundtrip diagnostic requires the legacy pipeline")
		return false
	}
	if scenarioConfig.IsRecordingEnable() && (stageRecording == nil || len(rule.GetActiveServices()) != 1) {
		logProtocolStageFallback(c, protocol.TypeOpenAIChat, target, "new recording path currently supports only single-service OpenAI Chat requests")
		return false
	}

	var requestErr error
	if stageRecording != nil {
		defer func() { stageRecording.finish(requestErr) }()
	}

	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeadersForTarget(c, target, rule, provider)
		c.Header(protocolPipelineHeader, "stage")
	}

	preBase := RulePreBaseTransforms(ruleFlags)
	preVendor := RulePreVendorTransforms(ruleFlags)
	disableStreamUsage := ruleFlags.SkipUsage || ruleFlags.CursorCompat
	terminal, registry, err := ph.protocolStageOpenAIChatTarget(
		target,
		provider,
		actualModel,
		responseModel,
		disableStreamUsage,
	)
	if err != nil {
		requestErr = err
		ph.FailAttemptSetup(c, err)
		return true
	}
	terminal = requestrecord.ObserveProvider(terminal, stageRecordingRecorder(stageRecording), requestrecord.ExchangeMetadata{
		Attempt:  1,
		Provider: provider.Name,
		Model:    actualModel,
	})

	scenarioFlags := scenarioFlagsOrNil(scenarioConfig)
	stages := []protocolstage.Stage{
		newProtocolTransformStage(
			"client_prepare",
			protocol.TypeOpenAIChat,
			provider,
			scenarioFlags,
			isStreaming,
			preBase,
			protocolStageTransformOptions(ph, c)...,
		),
		newProtocolTransformStage(
			"provider_finalize",
			target,
			provider,
			scenarioFlags,
			isStreaming,
			appendProtocolStageTransforms(
				[]transform.Transform{transform.NewConsistencyTransform(target)},
				preVendor,
				[]transform.Transform{vendorTransformShared},
			),
			protocolStageTransformOptions(ph, c)...,
		),
	}
	endpoint, err := protocolstage.BuildTopology(protocolstage.TopologyConfig{
		Terminal:             terminal,
		Stages:               stages,
		ClientProtocol:       protocol.TypeOpenAIChat,
		Registry:             registry,
		RequiredCapabilities: protocolstage.AllBridgeCapabilities,
	})
	if err != nil {
		requestErr = fmt.Errorf("build Protocol Stage topology: %w", err)
		ph.FailAttemptSetup(c, requestErr)
		return true
	}

	logProtocolStageEntry(c, protocol.TypeOpenAIChat, target, stages, isStreaming)

	call := protocolstage.Call{
		Request: req.ChatCompletionNewParams,
		Metadata: protocolstage.CallMetadata{
			RequestID: pkgobs.RequestIDFromContext(c.Request.Context()),
		},
	}
	if isStreaming {
		requestErr = ph.serveProtocolStageOpenAIChatStream(c, endpoint, call, responseModel, disableStreamUsage, nil, stageRecordingRecorder(stageRecording))
		return true
	}
	requestErr = ph.serveProtocolStageOpenAIChatComplete(c, endpoint, call, responseModel, provider, actualModel, disableStreamUsage, nil, stageRecordingRecorder(stageRecording))
	return true
}

func (ph *ProtocolHandler) protocolStageOpenAIChatTarget(
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	responseModel string,
	disableStreamUsage bool,
) (protocolstage.Endpoint, *protocolstage.BridgeRegistry, error) {
	registry, err := protocolstage.NewBridgeRegistry(
		protocolstage.NewIdentityBridge(protocol.TypeOpenAIChat),
		openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{
			DefaultMaxTokens:   4096,
			DisableStreamUsage: disableStreamUsage,
			ResponseModel:      responseModel,
		}),
		openaibridge.NewChatToOpenAIResponses(openaibridge.ResponsesOptions{
			DefaultMaxTokens:   4096,
			DisableStreamUsage: disableStreamUsage,
			ResponseModel:      responseModel,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build OpenAI Chat Protocol Stage registry: %w", err)
	}
	switch target {
	case protocol.TypeOpenAIChat:
		return &openAIChatProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeAnthropicBeta:
		return &anthropicBetaProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	default:
		return nil, nil, fmt.Errorf("OpenAI Chat Protocol Stage target %q is not implemented", target)
	}
}

func (ph *ProtocolHandler) shouldUseProtocolStage(
	c *gin.Context,
	source, target protocol.APIType,
	required protocolstage.Capabilities,
) bool {
	selector := ph.protocolStageSelector
	if selector == nil {
		return false
	}
	useStage, selectionErr := selector.ShouldUseStage(source, target, required)
	if !useStage && selector.Enabled() && selectionErr != nil {
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"protocol_pipeline": "legacy",
			"source_protocol":   source,
			"target_protocol":   target,
		}).Debugf("Protocol Stage route unavailable: %v", selectionErr)
	}
	return useStage
}

func logProtocolStageFallback(c *gin.Context, source, target protocol.APIType, reason string) {
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"protocol_pipeline": "legacy",
		"source_protocol":   source,
		"target_protocol":   target,
		"reason":            reason,
	}).Debug("Protocol Stage request stayed on legacy")
}

func logProtocolStageEntry(
	c *gin.Context,
	source, target protocol.APIType,
	stages []protocolstage.Stage,
	streaming bool,
) {
	operation := "complete"
	if streaming {
		operation = "stream"
	}
	chain := make([]string, 0, len(stages))
	for _, current := range stages {
		chain = append(chain, fmt.Sprintf("%s[%s]", current.Name(), current.Protocol()))
	}
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"protocol_pipeline": "stage",
		"source_protocol":   source,
		"target_protocol":   target,
		"operation":         operation,
		"stage_chain":       strings.Join(chain, " -> "),
	}).Debug("Entering Protocol Stage pipeline")
}

func appendProtocolStageTransforms(groups ...[]transform.Transform) []transform.Transform {
	var count int
	for _, group := range groups {
		count += len(group)
	}
	result := make([]transform.Transform, 0, count)
	for _, group := range groups {
		result = append(result, group...)
	}
	return result
}

func protocolStageTransformOptions(ph *ProtocolHandler, c *gin.Context) []transform.TransformOption {
	options := []transform.TransformOption{
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
	}
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		options = append(options, transform.WithIsAdvisorRequest(true))
	}
	return options
}

type protocolStageSetupError struct{ err error }

func (e *protocolStageSetupError) Error() string { return e.err.Error() }
func (e *protocolStageSetupError) Unwrap() error { return e.err }

// protocolTransformStage reuses the existing non-transport transforms inside a
// native Protocol Stage boundary. It is constructed per provider attempt, while
// all mutable transform state remains per call.
type protocolTransformStage struct {
	name          string
	api           protocol.APIType
	provider      *typ.Provider
	scenarioFlags *typ.ScenarioFlags
	streaming     bool
	transforms    []transform.Transform
	options       []transform.TransformOption
}

func newProtocolTransformStage(
	name string,
	api protocol.APIType,
	provider *typ.Provider,
	scenarioFlags *typ.ScenarioFlags,
	streaming bool,
	transforms []transform.Transform,
	options ...transform.TransformOption,
) protocolstage.Stage {
	return &protocolTransformStage{
		name:          name,
		api:           api,
		provider:      provider,
		scenarioFlags: scenarioFlags,
		streaming:     streaming,
		transforms:    append([]transform.Transform(nil), transforms...),
		options:       append([]transform.TransformOption(nil), options...),
	}
}

func (s *protocolTransformStage) Name() string               { return s.name }
func (s *protocolTransformStage) Protocol() protocol.APIType { return s.api }
func (s *protocolTransformStage) Wrap(next protocolstage.Endpoint) protocolstage.Endpoint {
	return &protocolTransformEndpoint{stage: s, next: next}
}

type protocolTransformEndpoint struct {
	stage *protocolTransformStage
	next  protocolstage.Endpoint
}

func (e *protocolTransformEndpoint) Protocol() protocol.APIType { return e.stage.api }

func (e *protocolTransformEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	prepared, release, err := e.stage.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	defer release()
	return e.next.Complete(ctx, prepared)
}

func (e *protocolTransformEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	prepared, release, err := e.stage.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	defer release()
	return e.next.Stream(ctx, prepared)
}

func (s *protocolTransformStage) prepare(ctx context.Context, call protocolstage.Call) (protocolstage.Call, func(), error) {
	if len(s.transforms) == 0 {
		return call, func() {}, nil
	}
	opts := []transform.TransformOption{
		transform.WithContext(ctx),
		transform.WithProvider(s.provider),
		transform.WithScenarioFlags(s.scenarioFlags),
		transform.WithStreaming(s.streaming),
	}
	opts = append(opts, s.options...)
	var transformCtx *transform.TransformContext
	switch request := call.Request.(type) {
	case *openai.ChatCompletionNewParams:
		transformCtx = transform.NewTransformContext(request, opts...)
		transformCtx.Config.OpenAIConfig = call.State.OpenAIChat
	case *responses.ResponseNewParams:
		transformCtx = transform.NewTransformContext(request, opts...)
	case *anthropic.MessageNewParams:
		transformCtx = transform.NewTransformContext(request, opts...)
	case *anthropic.BetaMessageNewParams:
		transformCtx = transform.NewTransformContext(request, opts...)
	default:
		return protocolstage.Call{}, func() {}, &protocolStageSetupError{err: fmt.Errorf(
			"Protocol Stage %q received request %T for %q",
			s.name,
			call.Request,
			s.api,
		)}
	}
	transformCtx.SourceAPI = s.api
	transformCtx.TargetAPI = s.api
	finalCtx, err := transform.NewTransformChain(s.transforms).Execute(transformCtx)
	if err != nil {
		transformCtx.Release()
		return protocolstage.Call{}, func() {}, &protocolStageSetupError{err: fmt.Errorf("Protocol Stage %q transform: %w", s.name, err)}
	}
	prepared := call
	prepared.Request = finalCtx.Request
	return prepared, finalCtx.Release, nil
}

// anthropicBetaProviderEndpoint is the transport-free provider terminal used
// by the first production Stage route.
type anthropicBetaProviderEndpoint struct {
	ph       *ProtocolHandler
	provider *typ.Provider
	model    string
}

func (*anthropicBetaProviderEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *anthropicBetaProviderEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	request, err := protocolStageBetaRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetAnthropicClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	message, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, request)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &protocolstage.Response{
		Value: message,
		Usage: protocolusage.FromAnthropicBetaMessage(message.Usage),
		Model: e.model,
	}, nil
}

func (e *anthropicBetaProviderEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	request, err := protocolStageBetaRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetAnthropicClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	stream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, request)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &anthropicBetaProviderStream{
		stream: stream,
		cancel: cancel,
		model:  e.model,
		usage:  protocolusage.NewAnthropicAccumulator(),
	}, nil
}

func protocolStageBetaRequest(value any) (*anthropic.BetaMessageNewParams, error) {
	request, ok := value.(*anthropic.BetaMessageNewParams)
	if !ok || request == nil {
		return nil, &protocolStageSetupError{err: fmt.Errorf("Anthropic Beta provider endpoint received %T", value)}
	}
	return request, nil
}

type anthropicBetaProviderStream struct {
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	cancel context.CancelFunc
	model  string
	usage  *protocolusage.AnthropicAccumulator

	closeOnce sync.Once
	closeErr  error
}

func (s *anthropicBetaProviderStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	if s.stream == nil {
		return protocolstage.Event{}, fmt.Errorf("Anthropic Beta provider stream is nil")
	}
	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil {
			return protocolstage.Event{}, err
		}
		return protocolstage.Event{}, io.EOF
	}
	event := s.stream.Current()
	if s.usage != nil {
		s.usage.ConsumeBeta(&event)
	}
	return protocolstage.Event{Value: event}, nil
}

func (s *anthropicBetaProviderStream) Close() error {
	s.closeOnce.Do(func() {
		if s.stream != nil {
			s.closeErr = s.stream.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
	})
	return s.closeErr
}

func (s *anthropicBetaProviderStream) Result() protocolstage.StreamResult {
	var usage *protocol.TokenUsage
	if s.usage != nil && s.usage.HasUsage() {
		usage = s.usage.Result()
	}
	return protocolstage.StreamResult{Usage: usage, Model: s.model}
}

func (ph *ProtocolHandler) serveProtocolStageOpenAIChatComplete(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	provider *typ.Provider,
	actualModel string,
	disableUsage bool,
	recorder *recording.ProtocolRecorder,
	requestRecorder *requestrecord.Recorder,
) error {
	response, err := endpoint.Complete(c.Request.Context(), call)
	if err != nil {
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return err
		}
		ph.failRequest(c, recorder, err, "Protocol Stage provider request failed")
		return err
	}
	value, err := protocolStageChatResponseMap(response.Value)
	if err != nil {
		ph.failRequest(c, recorder, err, "Protocol Stage response conversion failed")
		return err
	}
	value = ops.ApplyResponseTransforms(value, provider.APIBase, actualModel)
	value["model"] = responseModel
	if disableUsage {
		delete(value, "usage")
	}
	captureProtocolStageFinalResponse(c.Request.Context(), requestRecorder, protocol.TypeOpenAIChat, value)
	if response.Usage != nil && response.Usage.HasUsage() {
		ph.trackUsageWithTokenUsage(c, response.Usage, nil)
	}
	if recorder != nil {
		recorder.SetAssembledResponse(value)
		recorder.RecordResponse(provider, actualModel)
	}
	c.JSON(http.StatusOK, value)
	return nil
}

func protocolStageChatResponseMap(value any) (map[string]any, error) {
	switch response := value.(type) {
	case wire.ChatCompletionWire:
		return response.ToMap(), nil
	case *wire.ChatCompletionWire:
		if response == nil {
			return nil, fmt.Errorf("Protocol Stage OpenAI Chat response is nil")
		}
		return response.ToMap(), nil
	case openai.ChatCompletion:
		return protocolStageChatSDKMap(response)
	case *openai.ChatCompletion:
		if response == nil {
			return nil, fmt.Errorf("Protocol Stage OpenAI Chat response is nil")
		}
		return protocolStageChatSDKMap(response)
	default:
		return nil, fmt.Errorf("Protocol Stage OpenAI Chat response has type %T", value)
	}
}

func protocolStageChatSDKMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal Protocol Stage OpenAI Chat response: %w", err)
	}
	var result wire.ChatCompletionWire
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode Protocol Stage OpenAI Chat response: %w", err)
	}
	return result.ToMap(), nil
}

func (ph *ProtocolHandler) serveProtocolStageOpenAIChatStream(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	disableUsage bool,
	recorder *recording.ProtocolRecorder,
	requestRecorder *requestrecord.Recorder,
) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		err := fmt.Errorf("Protocol Stage streaming is unsupported by this connection")
		ph.FailAttemptSetup(c, err)
		return err
	}
	stream, err := endpoint.Stream(c.Request.Context(), call)
	if err != nil {
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return err
		}
		ph.failRequest(c, recorder, err, "Protocol Stage provider stream failed")
		return err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logrus.WithContext(c.Request.Context()).Warnf("close Protocol Stage stream: %v", closeErr)
		}
	}()
	finalCapture := newProtocolStageFinalStreamCapture(c.Request.Context(), requestRecorder, protocol.TypeOpenAIChat)

	wrote := false
	for {
		event, nextErr := stream.Next(c.Request.Context())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			result := stream.Result()
			if result.Usage != nil && result.Usage.HasUsage() {
				ph.trackUsageWithTokenUsage(c, result.Usage, nextErr)
			} else {
				ph.trackUsageFromContext(c, 0, 0, nextErr)
			}
			if !wrote {
				SendErrorResponse(c, nextErr, "Protocol Stage provider stream failed")
			} else {
				protocolstream.OpenAISSE(c, ErrorResponse{Error: ErrorDetail{
					Message: "Protocol Stage stream terminated",
					Type:    "protocol_error",
				}})
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(nextErr)
			}
			return nextErr
		}

		chunk, chunkErr := protocolStageChatStreamChunk(event.Value)
		if chunkErr != nil {
			streamErr := chunkErr
			result := stream.Result()
			if result.Usage != nil && result.Usage.HasUsage() {
				ph.trackUsageWithTokenUsage(c, result.Usage, streamErr)
			} else {
				ph.trackUsageFromContext(c, 0, 0, streamErr)
			}
			if !wrote {
				ph.FailAttemptSetup(c, streamErr)
			} else {
				protocolstream.OpenAISSE(c, ErrorResponse{Error: ErrorDetail{
					Message: "Protocol Stage stream emitted an invalid event",
					Type:    "protocol_error",
				}})
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(streamErr)
			}
			return streamErr
		}
		chunk.Model = responseModel
		if disableUsage {
			chunk.Usage = nil
			if len(chunk.Choices) == 0 {
				continue
			}
		}
		finalCapture.add(c.Request.Context(), chunk)
		if !wrote {
			setProtocolStageSSEHeaders(c)
			wrote = true
		}
		if protocolStageChatChunkHasContent(chunk) {
			protocol.MarkFirstToken(c)
		}
		protocolstream.OpenAISSE(c, chunk)
		flusher.Flush()
	}

	if !wrote {
		setProtocolStageSSEHeaders(c)
	}
	protocolstream.OpenAISSEDone(c)
	flusher.Flush()
	result := stream.Result()
	if result.Usage != nil && result.Usage.HasUsage() {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
	finalCapture.finish(c.Request.Context())
	return nil
}

func protocolStageChatStreamChunk(value any) (wire.ChatStreamChunk, error) {
	switch chunk := value.(type) {
	case wire.ChatStreamChunk:
		return chunk, nil
	case *wire.ChatStreamChunk:
		if chunk == nil {
			return wire.ChatStreamChunk{}, fmt.Errorf("Protocol Stage OpenAI Chat stream chunk is nil")
		}
		return *chunk, nil
	case openai.ChatCompletionChunk:
		return protocolStageChatSDKChunk(chunk)
	case *openai.ChatCompletionChunk:
		if chunk == nil {
			return wire.ChatStreamChunk{}, fmt.Errorf("Protocol Stage OpenAI Chat stream chunk is nil")
		}
		return protocolStageChatSDKChunk(chunk)
	default:
		return wire.ChatStreamChunk{}, fmt.Errorf("Protocol Stage stream emitted %T, want Chat stream chunk", value)
	}
}

func protocolStageChatSDKChunk(value any) (wire.ChatStreamChunk, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return wire.ChatStreamChunk{}, fmt.Errorf("marshal Protocol Stage OpenAI Chat stream chunk: %w", err)
	}
	var result wire.ChatStreamChunk
	if err := json.Unmarshal(raw, &result); err != nil {
		return wire.ChatStreamChunk{}, fmt.Errorf("decode Protocol Stage OpenAI Chat stream chunk: %w", err)
	}
	return result, nil
}

func setProtocolStageSSEHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
}

func protocolStageChatChunkHasContent(chunk wire.ChatStreamChunk) bool {
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" || choice.Delta.ReasoningContent != "" || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}
