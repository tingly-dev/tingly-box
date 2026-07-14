package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	protocolguardrail "github.com/tingly-dev/tingly-box/internal/protocol/stage/guardrail"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// tryProtocolStageAnthropicBeta selects an explicitly registered Beta-source
// route for one provider attempt. Anthropic V1 is intentionally not included:
// V1 and Beta remain distinct request, response, and streaming protocols.
func (ph *ProtocolHandler) tryProtocolStageAnthropicBeta(
	c *gin.Context,
	req *protocol.AnthropicBetaMessagesRequest,
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
	if !ph.shouldUseProtocolStage(
		c,
		protocol.TypeAnthropicBeta,
		target,
		protocolstage.AllBridgeCapabilities,
	) {
		return false
	}

	// These features still own parts of the legacy Beta lifecycle. Keep the
	// entire attempt on legacy until each one is represented by a native Stage;
	// partial execution would silently omit tool-loop or recording work.
	if ph.mcpEnabled() {
		logProtocolStageFallback(c, protocol.TypeAnthropicBeta, target, "MCP runtime still uses the legacy pipeline")
		return false
	}
	if recorder != nil || stageRecording != nil {
		if stageRecording == nil || target != protocol.TypeAnthropicBeta || len(rule.GetActiveServices()) != 1 {
			logProtocolStageFallback(c, protocol.TypeAnthropicBeta, target, "new recording canary currently supports only single-service Anthropic Beta identity")
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

	scenarioFlags, clientTransforms := protocolStageAnthropicBetaClientTransforms(scenarioConfig, ruleFlags)
	providerTransforms := appendProtocolStageTransforms(
		[]transform.Transform{transform.NewConsistencyTransform(target)},
		RulePreVendorTransforms(ruleFlags),
		[]transform.Transform{vendorTransformShared},
	)
	terminal, registry, err := ph.protocolStageAnthropicBetaTarget(
		target,
		provider,
		actualModel,
		responseModel,
		scenarioFlags,
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
	stages := []protocolstage.Stage{
		newProtocolTransformStage(
			"client_prepare",
			protocol.TypeAnthropicBeta,
			provider,
			scenarioFlags,
			isStreaming,
			clientTransforms,
			protocolStageTransformOptions(ph, c)...,
		),
	}
	if ph.guardrailsEnabledForScenario(GetTrackingContextScenario(c)) {
		guardrailStage, guardrailErr := protocolguardrail.NewAnthropicBeta(protocolguardrail.AnthropicBetaConfig{
			Runtime: ph.currentGuardrailsRuntime(),
			BaseInput: BuildGuardrailsBaseInput(
				c,
				actualModel,
				provider,
				guardrailscore.DirectionRequest,
				nil,
			),
			Observe: protocolStageGuardrailObserver(c),
		})
		if guardrailErr != nil {
			requestErr = guardrailErr
			ph.FailAttemptSetup(c, guardrailErr)
			return true
		}
		stages = append(stages, guardrailStage)
	}
	stages = append(stages,
		newProtocolTransformStage(
			"provider_finalize",
			target,
			provider,
			scenarioFlags,
			isStreaming,
			providerTransforms,
			protocolStageTransformOptions(ph, c)...,
		),
	)
	endpoint, err := protocolstage.BuildTopology(protocolstage.TopologyConfig{
		Terminal:             terminal,
		Stages:               stages,
		ClientProtocol:       protocol.TypeAnthropicBeta,
		Registry:             registry,
		RequiredCapabilities: protocolstage.AllBridgeCapabilities,
	})
	if err != nil {
		requestErr = fmt.Errorf("build Anthropic Beta Protocol Stage topology: %w", err)
		ph.FailAttemptSetup(c, requestErr)
		return true
	}

	logProtocolStageEntry(c, protocol.TypeAnthropicBeta, target, stages, isStreaming)

	call := protocolstage.Call{
		Request: req.BetaMessageNewParams,
		Metadata: protocolstage.CallMetadata{
			RequestID: pkgobs.RequestIDFromContext(c.Request.Context()),
		},
	}
	legacyRecorder := recorder
	if stageRecording != nil {
		// The canary emits one new-format envelope. The legacy recorder remains
		// the rollback path when Stage is disabled or this route falls back.
		legacyRecorder = nil
	}
	if isStreaming {
		requestErr = ph.serveProtocolStageAnthropicBetaStream(c, endpoint, call, responseModel, legacyRecorder, stageRecordingRecorder(stageRecording))
		return true
	}
	requestErr = ph.serveProtocolStageAnthropicBetaComplete(c, endpoint, call, responseModel, provider, actualModel, rule, legacyRecorder, stageRecordingRecorder(stageRecording))
	return true
}

func stageRecordingRecorder(recording *protocolStageRequestRecording) *requestrecord.Recorder {
	if recording == nil {
		return nil
	}
	return recording.recorder
}

func protocolStageGuardrailObserver(c *gin.Context) protocolguardrail.Observer {
	return func(observation protocolguardrail.Observation) {
		entry := logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"protocol_pipeline": "stage",
			"stage":             observation.Stage,
			"stage_protocol":    observation.Protocol,
			"guardrail_phase":   observation.Phase,
			"guardrail_verdict": observation.Decision.Verdict,
		})
		if observation.Err != nil {
			entry.WithError(observation.Err).Warn("Protocol Stage Guardrail evaluation failed open")
			return
		}
		if observation.Decision.Verdict == protocolguardrail.VerdictBlock {
			entry.Debug("Protocol Stage Guardrail changed the response")
		}
	}
}

func (ph *ProtocolHandler) protocolStageAnthropicBetaTarget(
	target protocol.APIType,
	provider *typ.Provider,
	actualModel string,
	responseModel string,
	scenarioFlags *typ.ScenarioFlags,
) (protocolstage.Endpoint, *protocolstage.BridgeRegistry, error) {
	disableStreamUsage := scenarioFlags != nil && scenarioFlags.SkipUsage
	betaToChat := anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{
		Compatible:         true,
		DisableStreamUsage: disableStreamUsage,
		ResponseModel:      responseModel,
	})
	betaToResponses := anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{
		ResponseModel: responseModel,
	})
	registry, err := protocolstage.NewBridgeRegistry(
		protocolstage.NewIdentityBridge(protocol.TypeAnthropicBeta),
		betaToChat,
		betaToResponses,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build Anthropic Beta Protocol Stage registry: %w", err)
	}

	switch target {
	case protocol.TypeAnthropicBeta:
		return &anthropicBetaProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIChat:
		return &openAIChatProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesProviderEndpoint{ph: ph, provider: provider, model: actualModel}, registry, nil
	default:
		return nil, nil, fmt.Errorf("Anthropic Beta Protocol Stage target %q is not implemented", target)
	}
}

func protocolStageAnthropicBetaClientTransforms(
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

func (ph *ProtocolHandler) serveProtocolStageAnthropicBetaComplete(
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
		ph.failRequest(c, recorder, err, "Anthropic Beta Protocol Stage provider request failed")
		return err
	}
	message, ok := response.Value.(*anthropic.BetaMessage)
	if !ok || message == nil {
		responseErr := fmt.Errorf("Anthropic Beta Protocol Stage response has type %T", response.Value)
		ph.failRequest(c, recorder, responseErr, "Protocol Stage response conversion failed")
		return responseErr
	}
	body, err := protocolStageAnthropicBetaMessageJSON(message, responseModel)
	if err != nil {
		ph.failRequest(c, recorder, err, "Protocol Stage response conversion failed")
		return err
	}
	if requestRecorder != nil {
		if captureErr := requestRecorder.SetFinalResponse(protocol.TypeAnthropicBeta, json.RawMessage(body)); captureErr != nil {
			logrus.WithContext(c.Request.Context()).WithError(captureErr).Debug("Protocol Stage RequestRecord final response capture failed")
		}
	}
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

func protocolStageAnthropicBetaMessageJSON(message *anthropic.BetaMessage, responseModel string) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("Anthropic Beta Protocol Stage response is nil")
	}
	structured, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic Beta response: %w", err)
	}
	var object map[string]json.RawMessage
	if raw := []byte(message.RawJSON()); len(raw) > 0 {
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, fmt.Errorf("decode Anthropic Beta raw response: %w", err)
		}
	} else {
		object = make(map[string]json.RawMessage)
	}
	// The SDK retains the provider wire payload in RawJSON. Merge current
	// structured fields over that payload so Stage mutations (Guardrails,
	// transforms) reach the client while unknown provider fields survive.
	var current map[string]json.RawMessage
	if err := json.Unmarshal(structured, &current); err != nil {
		return nil, fmt.Errorf("decode structured Anthropic Beta response: %w", err)
	}
	for key, value := range current {
		object[key] = value
	}
	model, err := json.Marshal(responseModel)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic Beta response model: %w", err)
	}
	object["model"] = model
	body, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic Beta response: %w", err)
	}
	return body, nil
}

func (ph *ProtocolHandler) serveProtocolStageAnthropicBetaStream(
	c *gin.Context,
	endpoint protocolstage.Endpoint,
	call protocolstage.Call,
	responseModel string,
	recorder *recording.ProtocolRecorder,
	requestRecorder *requestrecord.Recorder,
) error {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		err := fmt.Errorf("Anthropic Beta Protocol Stage streaming is unsupported by this connection")
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
		ph.failRequest(c, recorder, err, "Anthropic Beta Protocol Stage provider stream failed")
		return err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logrus.WithContext(c.Request.Context()).Warnf("close Anthropic Beta Protocol Stage stream: %v", closeErr)
		}
	}()
	var finalAssembler assembler.StreamAssembler
	if requestRecorder != nil {
		finalAssembler, err = assembler.NewStreamAssembler(protocol.TypeAnthropicBeta)
		if err != nil {
			logrus.WithContext(c.Request.Context()).WithError(err).Debug("Protocol Stage RequestRecord final stream assembler unavailable")
		}
	}

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
				SendErrorResponse(c, nextErr, "Anthropic Beta Protocol Stage provider stream failed")
			} else {
				protocolstream.MarshalAndSendErrorEvent(c, "Protocol Stage stream terminated", "stream_error", "stream_failed")
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(nextErr)
			}
			return nextErr
		}

		eventType, payload, eventErr := protocolStageAnthropicBetaEventJSON(event.Value, responseModel)
		if eventErr != nil {
			streamErr := fmt.Errorf("Anthropic Beta Protocol Stage stream emitted %T", event.Value)
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
		if finalAssembler != nil {
			if captureErr := finalAssembler.Add(json.RawMessage(payload)); captureErr != nil {
				logrus.WithContext(c.Request.Context()).WithError(captureErr).Debug("Protocol Stage RequestRecord final stream event capture failed")
				finalAssembler = nil
			}
		}
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
		return errors.New("Anthropic Beta Protocol Stage stream ended before message_stop")
	}
	result := stream.Result()
	if result.Usage != nil {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
	if finalAssembler != nil && requestRecorder != nil {
		finalResponse, captureErr := finalAssembler.Finish()
		if captureErr != nil {
			logrus.WithContext(c.Request.Context()).WithError(captureErr).Debug("Protocol Stage RequestRecord final stream assembly failed")
		} else if captureErr = requestRecorder.SetFinalResponse(protocol.TypeAnthropicBeta, finalResponse); captureErr != nil {
			logrus.WithContext(c.Request.Context()).WithError(captureErr).Debug("Protocol Stage RequestRecord final stream response capture failed")
		}
	}
	return nil
}

func protocolStageAnthropicBetaEventJSON(value any, responseModel string) (string, []byte, error) {
	var eventType string
	var raw []byte
	switch event := value.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		eventType = event.Type
		raw = []byte(event.RawJSON())
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(event)
			if err != nil {
				return "", nil, fmt.Errorf("marshal Anthropic Beta stream event: %w", err)
			}
		}
	case protocolstream.AnthropicEvent:
		eventType = event.Type
		var err error
		raw, err = json.Marshal(event.Data)
		if err != nil {
			return "", nil, fmt.Errorf("marshal converted Anthropic Beta stream event: %w", err)
		}
	default:
		return "", nil, fmt.Errorf("unsupported Anthropic Beta stream event %T", value)
	}
	if eventType == "" {
		return "", nil, fmt.Errorf("Anthropic Beta stream event has empty type")
	}
	if eventType != "message_start" {
		return eventType, raw, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", nil, fmt.Errorf("decode Anthropic Beta message_start: %w", err)
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(object["message"], &message); err != nil {
		return "", nil, fmt.Errorf("decode Anthropic Beta message_start.message: %w", err)
	}
	model, err := json.Marshal(responseModel)
	if err != nil {
		return "", nil, fmt.Errorf("marshal Anthropic Beta stream response model: %w", err)
	}
	message["model"] = model
	object["message"], err = json.Marshal(message)
	if err != nil {
		return "", nil, fmt.Errorf("encode Anthropic Beta message_start.message: %w", err)
	}
	payload, err := json.Marshal(object)
	if err != nil {
		return "", nil, fmt.Errorf("encode Anthropic Beta message_start: %w", err)
	}
	return eventType, payload, nil
}

func setProtocolStageAnthropicSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}
