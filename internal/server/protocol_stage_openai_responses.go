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

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/responsesbridge"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// tryProtocolStageOpenAIResponses selects an explicitly registered
// Responses-source route and owns its complete request lifecycle.
func (ph *ProtocolHandler) tryProtocolStageOpenAIResponses(
	c *gin.Context,
	req *protocol.ResponseCreateRequest,
	responseModel string,
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	rule *typ.Rule,
	isStreaming bool,
	scenarioConfig *typ.ScenarioConfig,
	ruleFlags typ.RuleFlags,
	maxAllowed int,
	stageRecording *protocolStageRequestRecording,
) bool {
	mcpEnabled := ph.mcpEnabled()
	if mcpEnabled {
		if !ph.shouldUseProtocolStageBetaChain(c, protocol.TypeOpenAIResponses, target, protocolstage.AllBridgeCapabilities) {
			return false
		}
		if ph.deps.MCPRuntime == nil {
			logProtocolStageFallback(c, protocol.TypeOpenAIResponses, target, "MCP runtime is unavailable")
			return false
		}
	} else if !ph.shouldUseProtocolStage(c, protocol.TypeOpenAIResponses, target, protocolstage.AllBridgeCapabilities) {
		return false
	}
	if scenarioConfig.IsRecordingEnable() && stageRecording == nil {
		logProtocolStageFallback(c, protocol.TypeOpenAIResponses, target, "new recording path requires a fully Stage-compatible service set")
		return false
	}

	var requestErr error
	if stageRecording != nil {
		defer func() { stageRecording.observeAttempt(requestErr) }()
	}

	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeadersForTarget(c, target, rule, provider)
		c.Header(protocolPipelineHeader, "stage")
	}

	scenarioFlags := scenarioFlagsOrNil(scenarioConfig)
	options := append(protocolStageTransformOptions(ph, c), transform.WithMaxTokens(int64(maxAllowed)))
	stages := []protocolstage.Stage{
		newProtocolTransformStage(
			"client_prepare",
			protocol.TypeOpenAIResponses,
			provider,
			scenarioFlags,
			isStreaming,
			RulePreBaseTransforms(ruleFlags),
			options...,
		),
	}
	if mcpEnabled {
		toolLoop, toolLoopErr := ph.newProtocolStageBetaToolLoop(c, provider, false)
		if toolLoopErr != nil {
			requestErr = toolLoopErr
			ph.FailAttemptSetup(c, toolLoopErr)
			return true
		}
		stages = append(stages, toolLoop)
	}
	stages = append(stages,
		newProtocolTransformStage(
			"provider_finalize",
			target,
			provider,
			scenarioFlags,
			isStreaming,
			appendProtocolStageTransforms(
				[]transform.Transform{transform.NewConsistencyTransform(target)},
				RulePreVendorTransforms(ruleFlags),
				[]transform.Transform{vendorTransformShared},
			),
			options...,
		),
	)
	terminal, registry, err := ph.protocolStageOpenAIResponsesTarget(target, provider, actualModel, responseModel, maxAllowed)
	if err != nil {
		requestErr = err
		ph.FailAttemptSetup(c, err)
		return true
	}
	terminal = requestrecord.ObserveProvider(terminal, stageRecordingRecorder(stageRecording), requestrecord.ExchangeMetadata{
		Attempt:  currentProtocolStageAttempt(c),
		Provider: provider.Name,
		Model:    actualModel,
	})
	endpoint, err := protocolstage.BuildTopology(protocolstage.TopologyConfig{
		Terminal:             terminal,
		Stages:               stages,
		ClientProtocol:       protocol.TypeOpenAIResponses,
		Registry:             registry,
		RequiredCapabilities: protocolstage.AllBridgeCapabilities,
	})
	if err != nil {
		requestErr = fmt.Errorf("build OpenAI Responses Protocol Stage topology: %w", err)
		ph.FailAttemptSetup(c, requestErr)
		return true
	}

	logProtocolStageEntry(c, protocol.TypeOpenAIResponses, target, stages, isStreaming)
	call := protocolstage.Call{
		Request: req.ResponseNewParams,
		Metadata: protocolstage.CallMetadata{
			RequestID: pkgobs.RequestIDFromContext(c.Request.Context()),
		},
	}
	if isStreaming {
		requestErr = ph.serveProtocolStageOpenAIResponsesStream(c, endpoint, call, responseModel, stageRecordingRecorder(stageRecording))
		return true
	}
	requestErr = ph.serveProtocolStageOpenAIResponsesComplete(c, endpoint, call, responseModel, stageRecordingRecorder(stageRecording))
	return true
}

func (ph *ProtocolHandler) protocolStageOpenAIResponsesTarget(
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	responseModel string,
	maxAllowed int,
) (protocolstage.Endpoint, *protocolstage.BridgeRegistry, error) {
	responsesToBeta := responsesbridge.NewToAnthropicBeta(responsesbridge.AnthropicOptions{
		DefaultMaxTokens: int64(maxAllowed),
		ResponseModel:    responseModel,
	})
	responsesToChat := responsesbridge.NewToOpenAIChat(responsesbridge.ChatOptions{
		DefaultMaxTokens: int64(maxAllowed),
		ResponseModel:    responseModel,
	})
	registry, err := protocolstage.NewBridgeRegistry(
		protocolstage.NewIdentityBridge(protocol.TypeOpenAIResponses),
		protocolstage.NewIdentityBridge(protocol.TypeAnthropicBeta),
		responsesToBeta,
		responsesToChat,
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{
			Compatible:    true,
			ResponseModel: responseModel,
		}),
		anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{
			ResponseModel: responseModel,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build OpenAI Responses Protocol Stage registry: %w", err)
	}
	switch target {
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeAnthropicBeta:
		return &anthropicBetaProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIChat:
		return &openAIChatProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	default:
		return nil, nil, fmt.Errorf("OpenAI Responses Protocol Stage target %q is not implemented", target)
	}
}

// openAIResponsesProviderEndpoint is the transport-free Responses provider
// terminal. HTTP parsing, headers, SSE framing, and public model rewriting stay
// at the outer server adapter.
type openAIResponsesProviderEndpoint struct {
	ph       *ProtocolHandler
	provider *typ.Provider
	model    string
}

func (*openAIResponsesProviderEndpoint) Protocol() protocol.APIType {
	return protocol.TypeOpenAIResponses
}

func (e *openAIResponsesProviderEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	request, err := protocolStageOpenAIResponsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetOpenAIClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	response, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, *request)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &protocolstage.Response{
		Value: response,
		Usage: protocolusage.FromOpenAIResponses(response.Usage),
		Model: e.model,
	}, nil
}

func (e *openAIResponsesProviderEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	request, err := protocolStageOpenAIResponsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetOpenAIClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	stream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, *request)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	primed, err := protocolstream.PrimeResponsesStream(stream)
	if err != nil {
		if stream != nil {
			_ = stream.Close()
		}
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &openAIResponsesProviderStream{
		stream: primed,
		cancel: cancel,
		model:  e.model,
	}, nil
}

func protocolStageOpenAIResponsesRequest(value any) (*responses.ResponseNewParams, error) {
	request, ok := value.(*responses.ResponseNewParams)
	if !ok || request == nil {
		return nil, &protocolStageSetupError{err: fmt.Errorf("OpenAI Responses provider endpoint received %T", value)}
	}
	return request, nil
}

type openAIResponsesProviderStream struct {
	stream protocolstream.ResponsesStreamIter
	cancel context.CancelFunc
	model  string
	usage  *protocol.TokenUsage

	closeOnce sync.Once
	closeErr  error
}

func (s *openAIResponsesProviderStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	if s.stream == nil {
		return protocolstage.Event{}, fmt.Errorf("OpenAI Responses provider stream is nil")
	}
	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil {
			return protocolstage.Event{}, err
		}
		return protocolstage.Event{}, io.EOF
	}
	event := s.stream.Current()
	if usage := protocolStageOpenAIResponsesUsage([]byte(event.RawJSON())); usage != nil {
		s.usage = usage
	}
	return protocolstage.Event{Value: event}, nil
}

func (s *openAIResponsesProviderStream) Close() error {
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

func (s *openAIResponsesProviderStream) Result() protocolstage.StreamResult {
	return protocolstage.StreamResult{Usage: s.usage, Model: s.model}
}

func (ph *ProtocolHandler) serveProtocolStageOpenAIResponsesComplete(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	requestRecorder *requestrecord.Recorder,
) error {
	result, err := endpoint.Complete(c.Request.Context(), call)
	if err != nil {
		preserveProtocolStageSideEffectBoundary(c, err, false)
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return err
		}
		ph.failRequest(c, nil, err, "OpenAI Responses Protocol Stage provider request failed")
		return err
	}
	preserveProtocolStageSideEffectBoundary(c, nil, result.SideEffectsCommitted)
	body, err := protocolStageOpenAIResponsesValueJSON(result.Value, responseModel)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return err
	}
	captureProtocolStageFinalResponse(c.Request.Context(), requestRecorder, protocol.TypeOpenAIResponses, json.RawMessage(body))
	if result.Usage != nil {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return nil
}

func protocolStageOpenAIResponsesValueJSON(value any, responseModel string) ([]byte, error) {
	switch response := value.(type) {
	case *responses.Response:
		return protocolStageOpenAIResponsesJSON(response, responseModel)
	case responses.Response:
		return protocolStageOpenAIResponsesJSON(&response, responseModel)
	case wire.ResponsesWireResponse:
		response.Model = responseModel
		body, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("marshal Protocol Stage Responses wire response: %w", err)
		}
		return body, nil
	case *wire.ResponsesWireResponse:
		if response == nil {
			return nil, fmt.Errorf("OpenAI Responses Protocol Stage wire response is nil")
		}
		converted := *response
		converted.Model = responseModel
		body, err := json.Marshal(converted)
		if err != nil {
			return nil, fmt.Errorf("marshal Protocol Stage Responses wire response: %w", err)
		}
		return body, nil
	default:
		return nil, fmt.Errorf("OpenAI Responses Protocol Stage response has type %T", value)
	}
}

func protocolStageOpenAIResponsesJSON(response *responses.Response, responseModel string) ([]byte, error) {
	if response == nil {
		return nil, fmt.Errorf("OpenAI Responses Protocol Stage response is nil")
	}
	body := []byte(response.RawJSON())
	if len(body) == 0 {
		var err error
		body, err = json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("marshal OpenAI Responses response: %w", err)
		}
	}
	modified, err := sjson.SetBytes(body, "model", responseModel)
	if err != nil {
		return nil, fmt.Errorf("rewrite OpenAI Responses response model: %w", err)
	}
	return modified, nil
}

func (ph *ProtocolHandler) serveProtocolStageOpenAIResponsesStream(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	requestRecorder *requestrecord.Recorder,
) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		err := fmt.Errorf("OpenAI Responses Protocol Stage streaming is unsupported by this connection")
		ph.FailAttemptSetup(c, err)
		return err
	}
	stream, err := endpoint.Stream(c.Request.Context(), call)
	if err != nil {
		preserveProtocolStageSideEffectBoundary(c, err, false)
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return err
		}
		ph.handlePreStreamFailure(c, err, nil)
		return err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logrus.WithContext(c.Request.Context()).Warnf("close OpenAI Responses Protocol Stage stream: %v", closeErr)
		}
	}()
	finalCapture := newProtocolStageFinalStreamCapture(c.Request.Context(), requestRecorder, protocol.TypeOpenAIResponses)

	wrote := false
	for {
		event, nextErr := stream.Next(c.Request.Context())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			result := stream.Result()
			preserveProtocolStageSideEffectBoundary(c, nextErr, result.SideEffectsCommitted)
			if errors.Is(nextErr, context.Canceled) || protocol.IsContextCanceled(nextErr) {
				if result.Usage != nil {
					ph.trackUsageWithTokenUsage(c, result.Usage, nil)
				}
				return nextErr
			}
			if result.Usage != nil && result.Usage.HasUsage() {
				ph.trackUsageWithTokenUsage(c, result.Usage, nextErr)
			} else {
				ph.trackUsageFromContext(c, 0, 0, nextErr)
			}
			if !wrote {
				protocolstream.SendStreamingError(c, nextErr)
			} else {
				protocolstream.OpenAIResponsesEvent(c, "error", map[string]any{
					"error": map[string]any{
						"message": nextErr.Error(),
						"type":    "stream_error",
						"code":    "stream_failed",
					},
				})
				flusher.Flush()
			}
			return nextErr
		}

		eventType, payload, eventErr := protocolStageOpenAIResponsesEventJSON(event.Value, responseModel)
		if eventErr != nil {
			streamErr := fmt.Errorf("OpenAI Responses Protocol Stage stream emitted %T: %w", event.Value, eventErr)
			preserveProtocolStageSideEffectBoundary(c, streamErr, stream.Result().SideEffectsCommitted)
			if !wrote {
				ph.FailAttemptSetup(c, streamErr)
			} else {
				protocolstream.OpenAIResponsesEvent(c, "error", map[string]any{
					"error": map[string]any{
						"message": "Protocol Stage stream emitted an invalid event",
						"type":    "stream_error",
						"code":    "stream_failed",
					},
				})
				flusher.Flush()
			}
			return streamErr
		}
		finalCapture.add(c.Request.Context(), json.RawMessage(payload))
		if !wrote {
			setProtocolStageOpenAIResponsesSSEHeaders(c)
			wrote = true
		}
		protocolstream.OpenAIResponsesEvent(c, eventType, payload)
		flusher.Flush()
	}

	result := stream.Result()
	preserveProtocolStageSideEffectBoundary(c, nil, result.SideEffectsCommitted)
	if result.Usage != nil {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
	finalCapture.finish(c.Request.Context())
	return nil
}

func setProtocolStageOpenAIResponsesSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}

func protocolStageOpenAIResponsesEventJSON(value any, responseModel string) (string, []byte, error) {
	var eventType string
	var raw []byte
	switch event := value.(type) {
	case responses.ResponseStreamEventUnion:
		eventType = event.Type
		raw = []byte(strings.Clone(event.RawJSON()))
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(event)
			if err != nil {
				return "", nil, fmt.Errorf("marshal OpenAI Responses stream event: %w", err)
			}
		}
	case wire.ResponsesEvent:
		eventType = event.EventType()
		var err error
		raw, err = json.Marshal(event)
		if err != nil {
			return "", nil, fmt.Errorf("marshal converted OpenAI Responses stream event: %w", err)
		}
	default:
		return "", nil, fmt.Errorf("unsupported OpenAI Responses stream event %T", value)
	}
	if eventType == "" {
		return "", nil, fmt.Errorf("OpenAI Responses stream event has empty type")
	}

	response := gjson.GetBytes(raw, "response")
	if !response.Exists() {
		return eventType, raw, nil
	}
	usage := response.Get("usage")
	if inputDetails := usage.Get("input_tokens_details"); inputDetails.Exists() && !inputDetails.Get("cached_tokens").Exists() {
		modified, err := sjson.SetBytes(raw, "response.usage.input_tokens_details.cached_tokens", 0)
		if err != nil {
			return "", nil, fmt.Errorf("backfill OpenAI Responses cached tokens: %w", err)
		}
		raw = modified
	}
	if usage.Exists() && !usage.Get("output_tokens_details.reasoning_tokens").Exists() {
		modified, err := sjson.SetBytes(raw, "response.usage.output_tokens_details.reasoning_tokens", 0)
		if err != nil {
			return "", nil, fmt.Errorf("backfill OpenAI Responses reasoning tokens: %w", err)
		}
		raw = modified
	}
	if model := response.Get("model"); model.Exists() && model.String() != "" {
		modified, err := sjson.SetBytes(raw, "response.model", responseModel)
		if err != nil {
			return "", nil, fmt.Errorf("rewrite OpenAI Responses stream model: %w", err)
		}
		raw = modified
	}
	return eventType, raw, nil
}

func protocolStageOpenAIResponsesUsage(raw []byte) *protocol.TokenUsage {
	response := gjson.GetBytes(raw, "response")
	if !response.Exists() {
		return nil
	}
	usage := response.Get("usage")
	input := usage.Get("input_tokens").Int()
	output := usage.Get("output_tokens").Int()
	cache := usage.Get("input_tokens_details.cached_tokens").Int()
	reasoning := usage.Get("output_tokens_details.reasoning_tokens").Int()
	if input == 0 && output == 0 && cache == 0 && reasoning == 0 {
		return nil
	}
	return protocol.NewTokenUsageFull(int(input-cache), int(output), int(cache), int(reasoning))
}
