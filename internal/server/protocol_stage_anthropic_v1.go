package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// tryProtocolStageAnthropicV1 selects only explicitly registered V1-source
// routes. It deliberately does not reinterpret a V1 request as Anthropic Beta.
func (ph *ProtocolHandler) tryProtocolStageAnthropicV1(
	c *gin.Context,
	req *protocol.AnthropicMessagesRequest,
	responseModel string,
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	rule *typ.Rule,
	isStreaming bool,
	scenarioConfig *typ.ScenarioConfig,
	ruleFlags typ.RuleFlags,
	recorder *recording.ProtocolRecorder,
	stageRecording *protocolStageRequestRecording,
) bool {
	if !ph.shouldUseProtocolStage(c, protocol.TypeAnthropicV1, target, protocolstage.AllBridgeCapabilities) {
		return false
	}
	if ph.mcpEnabled() {
		logProtocolStageFallback(c, protocol.TypeAnthropicV1, target, "MCP runtime still uses the legacy pipeline")
		return false
	}
	if ph.guardrailsEnabledForScenario(GetTrackingContextScenario(c)) {
		logProtocolStageFallback(c, protocol.TypeAnthropicV1, target, "Guardrails still use the legacy Anthropic V1 lifecycle")
		return false
	}
	if recorder != nil || stageRecording != nil {
		if stageRecording == nil || len(rule.GetActiveServices()) != 1 {
			logProtocolStageFallback(c, protocol.TypeAnthropicV1, target, "new recording path currently supports only single-service Anthropic V1 requests")
			return false
		}
	}

	var requestErr error
	if stageRecording != nil {
		defer func() { stageRecording.finish(requestErr) }()
	}

	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeadersForTarget(c, target, rule, provider)
		c.Header(protocolPipelineHeader, "stage")
	}

	scenarioFlags, clientTransforms := protocolStageAnthropicV1ClientTransforms(scenarioConfig, ruleFlags)
	providerTransforms := appendProtocolStageTransforms(
		[]transform.Transform{transform.NewConsistencyTransform(target)},
		RulePreVendorTransforms(ruleFlags),
		[]transform.Transform{vendorTransformShared},
	)
	terminal, registry, err := ph.protocolStageAnthropicV1Target(target, provider, actualModel, responseModel, scenarioFlags)
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
	stages := []protocolstage.Stage{
		newProtocolTransformStage(
			"client_prepare",
			protocol.TypeAnthropicV1,
			provider,
			scenarioFlags,
			isStreaming,
			clientTransforms,
			protocolStageTransformOptions(ph, c)...,
		),
		newProtocolTransformStage(
			"provider_finalize",
			target,
			provider,
			scenarioFlags,
			isStreaming,
			providerTransforms,
			protocolStageTransformOptions(ph, c)...,
		),
	}
	endpoint, err := protocolstage.BuildTopology(protocolstage.TopologyConfig{
		Terminal:             terminal,
		Stages:               stages,
		ClientProtocol:       protocol.TypeAnthropicV1,
		Registry:             registry,
		RequiredCapabilities: protocolstage.AllBridgeCapabilities,
	})
	if err != nil {
		requestErr = fmt.Errorf("build Anthropic V1 Protocol Stage topology: %w", err)
		ph.FailAttemptSetup(c, requestErr)
		return true
	}

	logProtocolStageEntry(c, protocol.TypeAnthropicV1, target, stages, isStreaming)

	call := protocolstage.Call{
		Request: req.MessageNewParams,
		Metadata: protocolstage.CallMetadata{
			RequestID: pkgobs.RequestIDFromContext(c.Request.Context()),
		},
	}
	legacyRecorder := recorder
	if stageRecording != nil {
		legacyRecorder = nil
	}
	if isStreaming {
		requestErr = ph.serveProtocolStageAnthropicV1Stream(c, endpoint, call, responseModel, legacyRecorder, stageRecordingRecorder(stageRecording))
		return true
	}
	requestErr = ph.serveProtocolStageAnthropicV1Complete(c, endpoint, call, responseModel, provider, actualModel, rule, legacyRecorder, stageRecordingRecorder(stageRecording))
	return true
}

func (ph *ProtocolHandler) protocolStageAnthropicV1Target(
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	responseModel string,
	scenarioFlags *typ.ScenarioFlags,
) (protocolstage.Endpoint, *protocolstage.BridgeRegistry, error) {
	disableStreamUsage := scenarioFlags != nil && scenarioFlags.SkipUsage
	registry, err := protocolstage.NewBridgeRegistry(
		protocolstage.NewIdentityBridge(protocol.TypeAnthropicV1),
		anthropicbridge.NewV1ToOpenAIChat(anthropicbridge.ChatOptions{
			Compatible:         true,
			DisableStreamUsage: disableStreamUsage,
			ResponseModel:      responseModel,
		}),
		anthropicbridge.NewV1ToOpenAIResponses(anthropicbridge.ResponsesOptions{
			ResponseModel: responseModel,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build Anthropic V1 Protocol Stage registry: %w", err)
	}
	switch target {
	case protocol.TypeAnthropicV1:
		return &anthropicV1ProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIChat:
		return &openAIChatProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	default:
		return nil, nil, fmt.Errorf("Anthropic V1 Protocol Stage target %q is not implemented", target)
	}
}

func protocolStageAnthropicV1ClientTransforms(
	scenarioConfig *typ.ScenarioConfig,
	ruleFlags typ.RuleFlags,
) (*typ.ScenarioFlags, []transform.Transform) {
	var scenarioFlags *typ.ScenarioFlags
	var transforms []transform.Transform
	if scenarioConfig != nil {
		flags := scenarioConfig.GetDefaultFlags()
		scenarioFlags = &flags
		if flags.SmartCompact {
			transforms = append(transforms, servertransform.NewThinkingCompactTransform(2))
		}
	}
	transforms = append(transforms, RulePreBaseTransforms(ruleFlags)...)
	return scenarioFlags, transforms
}

type anthropicV1ProviderEndpoint struct {
	ph       *ProtocolHandler
	provider *typ.Provider
	model    string
}

func (*anthropicV1ProviderEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicV1 }

func (e *anthropicV1ProviderEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	request, err := protocolStageAnthropicV1Request(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetAnthropicClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	message, cancel, err := forwarding.ForwardAnthropicV1(fc, wrapper, request)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		return nil, err
	}
	return &protocolstage.Response{
		Value: message,
		Usage: protocolusage.FromAnthropicMessage(message.Usage),
		Model: e.model,
	}, nil
}

func (e *anthropicV1ProviderEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	request, err := protocolStageAnthropicV1Request(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.ph.deps.ClientPool.GetAnthropicClient(ctx, e.provider, e.model)
	fc := forwarding.NewForwardContext(ctx, e.provider)
	stream, cancel, err := forwarding.ForwardAnthropicV1Stream(fc, wrapper, request)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &anthropicV1ProviderStream{
		stream: stream,
		cancel: cancel,
		model:  e.model,
		usage:  protocolusage.NewAnthropicAccumulator(),
	}, nil
}

func protocolStageAnthropicV1Request(value any) (*anthropic.MessageNewParams, error) {
	request, ok := value.(*anthropic.MessageNewParams)
	if !ok || request == nil {
		return nil, &protocolStageSetupError{err: fmt.Errorf("Anthropic V1 provider endpoint received %T", value)}
	}
	return request, nil
}

type anthropicV1ProviderStream struct {
	stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion]
	cancel context.CancelFunc
	model  string
	usage  *protocolusage.AnthropicAccumulator

	closeOnce sync.Once
	closeErr  error
}

func (s *anthropicV1ProviderStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	if s.stream == nil {
		return protocolstage.Event{}, fmt.Errorf("Anthropic V1 provider stream is nil")
	}
	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil {
			return protocolstage.Event{}, err
		}
		return protocolstage.Event{}, io.EOF
	}
	event := s.stream.Current()
	if s.usage != nil {
		s.usage.Consume(&event)
	}
	return protocolstage.Event{Value: event}, nil
}

func (s *anthropicV1ProviderStream) Close() error {
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

func (s *anthropicV1ProviderStream) Result() protocolstage.StreamResult {
	var usage *protocol.TokenUsage
	if s.usage != nil && s.usage.HasUsage() {
		usage = s.usage.Result()
	}
	return protocolstage.StreamResult{Usage: usage, Model: s.model}
}

func (ph *ProtocolHandler) serveProtocolStageAnthropicV1Complete(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	provider *typ.Provider,
	actualModel string,
	rule *typ.Rule,
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
		ph.failRequest(c, recorder, err, "Anthropic V1 Protocol Stage provider request failed")
		return err
	}
	message, ok := response.Value.(*anthropic.Message)
	if !ok || message == nil {
		responseErr := fmt.Errorf("Anthropic V1 Protocol Stage response has type %T", response.Value)
		ph.failRequest(c, recorder, responseErr, "Protocol Stage response conversion failed")
		return responseErr
	}
	body, err := protocolStageAnthropicV1MessageJSON(message, responseModel)
	if err != nil {
		ph.failRequest(c, recorder, err, "Protocol Stage response conversion failed")
		return err
	}
	captureProtocolStageFinalResponse(c.Request.Context(), requestRecorder, protocol.TypeAnthropicV1, json.RawMessage(body))
	if response.Usage != nil {
		ph.trackUsageWithTokenUsage(c, response.Usage, nil)
	}
	ph.updateAffinityMessageID(c, rule, string(message.ID))
	message.Model = anthropic.Model(responseModel)
	if recorder != nil {
		recorder.SetAssembledResponse(message)
		recorder.RecordResponse(provider, actualModel)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return nil
}

func protocolStageAnthropicV1MessageJSON(message *anthropic.Message, responseModel string) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("Anthropic V1 Protocol Stage response is nil")
	}
	raw := []byte(message.RawJSON())
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("marshal Anthropic V1 response: %w", err)
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode Anthropic V1 response: %w", err)
	}
	model, err := json.Marshal(responseModel)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic V1 response model: %w", err)
	}
	object["model"] = model
	body, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic V1 response: %w", err)
	}
	return body, nil
}

func (ph *ProtocolHandler) serveProtocolStageAnthropicV1Stream(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	recorder *recording.ProtocolRecorder,
	requestRecorder *requestrecord.Recorder,
) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		err := fmt.Errorf("Anthropic V1 Protocol Stage streaming is unsupported by this connection")
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
		ph.failRequest(c, recorder, err, "Anthropic V1 Protocol Stage provider stream failed")
		return err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logrus.WithContext(c.Request.Context()).Warnf("close Anthropic V1 Protocol Stage stream: %v", closeErr)
		}
	}()
	finalCapture := newProtocolStageFinalStreamCapture(c.Request.Context(), requestRecorder, protocol.TypeAnthropicV1)

	wrote := false
	sawMessageStart := false
	sawMessageStop := false
	for {
		event, nextErr := stream.Next(c.Request.Context())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			result := stream.Result()
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
				SendErrorResponse(c, nextErr, "Anthropic V1 Protocol Stage provider stream failed")
			} else {
				protocolstream.MarshalAndSendErrorEvent(c, "Protocol Stage stream terminated", "stream_error", "stream_failed")
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(nextErr)
			}
			return nextErr
		}

		eventType, payload, eventErr := protocolStageAnthropicV1EventJSON(event.Value, responseModel)
		if eventErr != nil {
			streamErr := fmt.Errorf("Anthropic V1 Protocol Stage stream emitted %T", event.Value)
			if !wrote {
				ph.FailAttemptSetup(c, errors.Join(streamErr, eventErr))
			} else {
				protocolstream.MarshalAndSendErrorEvent(c, "Protocol Stage stream emitted an invalid event", "stream_error", "stream_failed")
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(streamErr)
			}
			return errors.Join(streamErr, eventErr)
		}
		finalCapture.add(c.Request.Context(), json.RawMessage(payload))
		if !wrote {
			setProtocolStageAnthropicSSEHeaders(c)
			wrote = true
		}
		switch eventType {
		case "message_start":
			sawMessageStart = true
		case "message_stop":
			sawMessageStop = true
		case "content_block_delta":
			protocol.MarkFirstToken(c)
		}
		c.SSEvent(eventType, string(payload))
		flusher.Flush()
	}

	if !wrote {
		setProtocolStageAnthropicSSEHeaders(c)
	}
	if sawMessageStart && !sawMessageStop {
		protocolstream.MarshalAndSendErrorEvent(c, "upstream stream ended before completion", "stream_error", "incomplete_stream")
		flusher.Flush()
		return errors.New("Anthropic V1 Protocol Stage stream ended before message_stop")
	}
	result := stream.Result()
	if result.Usage != nil {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
	finalCapture.finish(c.Request.Context())
	return nil
}

func protocolStageAnthropicV1EventJSON(value any, responseModel string) (string, []byte, error) {
	var eventType string
	var raw []byte
	switch event := value.(type) {
	case anthropic.MessageStreamEventUnion:
		eventType = event.Type
		raw = []byte(event.RawJSON())
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(event)
			if err != nil {
				return "", nil, fmt.Errorf("marshal Anthropic V1 stream event: %w", err)
			}
		}
	case protocolstream.AnthropicEvent:
		eventType = event.Type
		var err error
		raw, err = json.Marshal(event.Data)
		if err != nil {
			return "", nil, fmt.Errorf("marshal converted Anthropic V1 stream event: %w", err)
		}
	default:
		return "", nil, fmt.Errorf("unsupported Anthropic V1 stream event %T", value)
	}
	if eventType == "" {
		return "", nil, fmt.Errorf("Anthropic V1 stream event has empty type")
	}
	if eventType != "message_start" {
		return eventType, raw, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", nil, fmt.Errorf("decode Anthropic V1 message_start: %w", err)
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(object["message"], &message); err != nil {
		return "", nil, fmt.Errorf("decode Anthropic V1 message_start.message: %w", err)
	}
	model, err := json.Marshal(responseModel)
	if err != nil {
		return "", nil, fmt.Errorf("marshal Anthropic V1 stream response model: %w", err)
	}
	message["model"] = model
	object["message"], err = json.Marshal(message)
	if err != nil {
		return "", nil, fmt.Errorf("encode Anthropic V1 message_start.message: %w", err)
	}
	payload, err := json.Marshal(object)
	if err != nil {
		return "", nil, fmt.Errorf("encode Anthropic V1 message_start: %w", err)
	}
	return eventType, payload, nil
}
