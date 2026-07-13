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
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// tryProtocolStageAnthropicBeta selects the explicitly registered native Beta
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
	// partial execution would silently omit response policy or tool-loop work.
	if ph.mcpEnabled() {
		logProtocolStageFallback(c, protocol.TypeAnthropicBeta, target, "MCP runtime still uses the legacy pipeline")
		return false
	}
	if ph.guardrailsEnabledForScenario(GetTrackingContextScenario(c)) {
		logProtocolStageFallback(c, protocol.TypeAnthropicBeta, target, "Guardrails still use the legacy Anthropic Beta lifecycle")
		return false
	}
	if recorder != nil {
		logProtocolStageFallback(c, protocol.TypeAnthropicBeta, target, "protocol recording still uses legacy transform and stream hooks")
		return false
	}

	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeadersForTarget(c, target, rule, provider)
		c.Header(protocolPipelineHeader, "stage")
	}

	scenarioFlags, clientTransforms := protocolStageAnthropicBetaClientTransforms(scenarioConfig, ruleFlags)
	providerTransforms := appendProtocolStageTransforms(
		[]transform.Transform{transform.NewConsistencyTransform(protocol.TypeAnthropicBeta)},
		RulePreVendorTransforms(ruleFlags),
		[]transform.Transform{vendorTransformShared},
	)
	registry, err := protocolstage.NewBridgeRegistry(protocolstage.NewIdentityBridge(protocol.TypeAnthropicBeta))
	if err != nil {
		ph.FailAttemptSetup(c, fmt.Errorf("build Anthropic Beta Protocol Stage registry: %w", err))
		return true
	}
	terminal := &anthropicBetaProviderEndpoint{ph: ph, provider: provider, model: actualModel}
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
		newProtocolTransformStage(
			"provider_finalize",
			protocol.TypeAnthropicBeta,
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
		ClientProtocol:       protocol.TypeAnthropicBeta,
		Registry:             registry,
		RequiredCapabilities: protocolstage.AllBridgeCapabilities,
	})
	if err != nil {
		ph.FailAttemptSetup(c, fmt.Errorf("build Anthropic Beta Protocol Stage topology: %w", err))
		return true
	}

	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"protocol_pipeline": "stage",
		"source_protocol":   protocol.TypeAnthropicBeta,
		"target_protocol":   target,
	}).Debug("Selected Protocol Stage pipeline")

	call := protocolstage.Call{
		Request: req.BetaMessageNewParams,
		Metadata: protocolstage.CallMetadata{
			RequestID: pkgobs.RequestIDFromContext(c.Request.Context()),
		},
	}
	if isStreaming {
		ph.serveProtocolStageAnthropicBetaStream(c, endpoint, call, responseModel, recorder)
		return true
	}
	ph.serveProtocolStageAnthropicBetaComplete(c, endpoint, call, responseModel, provider, actualModel, rule, recorder)
	return true
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
) {
	response, err := endpoint.Complete(c.Request.Context(), call)
	if err != nil {
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return
		}
		ph.failRequest(c, recorder, err, "Anthropic Beta Protocol Stage provider request failed")
		return
	}
	message, ok := response.Value.(*anthropic.BetaMessage)
	if !ok || message == nil {
		ph.failRequest(c, recorder, fmt.Errorf("Anthropic Beta Protocol Stage response has type %T", response.Value), "Protocol Stage response conversion failed")
		return
	}
	body, err := protocolStageAnthropicBetaMessageJSON(message, responseModel)
	if err != nil {
		ph.failRequest(c, recorder, err, "Protocol Stage response conversion failed")
		return
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
}

func protocolStageAnthropicBetaMessageJSON(message *anthropic.BetaMessage, responseModel string) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("Anthropic Beta Protocol Stage response is nil")
	}
	raw := []byte(message.RawJSON())
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("marshal Anthropic Beta response: %w", err)
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode Anthropic Beta response: %w", err)
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
) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		ph.FailAttemptSetup(c, fmt.Errorf("Anthropic Beta Protocol Stage streaming is unsupported by this connection"))
		return
	}
	stream, err := endpoint.Stream(c.Request.Context(), call)
	if err != nil {
		var setupErr *protocolStageSetupError
		if errors.As(err, &setupErr) {
			ph.FailAttemptSetup(c, setupErr)
			return
		}
		ph.failRequest(c, recorder, err, "Anthropic Beta Protocol Stage provider stream failed")
		return
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logrus.WithContext(c.Request.Context()).Warnf("close Anthropic Beta Protocol Stage stream: %v", closeErr)
		}
	}()

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
				return
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
			return
		}

		betaEvent, ok := event.Value.(anthropic.BetaRawMessageStreamEventUnion)
		if !ok {
			streamErr := fmt.Errorf("Anthropic Beta Protocol Stage stream emitted %T", event.Value)
			if !wrote {
				ph.FailAttemptSetup(c, streamErr)
			} else {
				protocolstream.MarshalAndSendErrorEvent(c, "Protocol Stage stream emitted an invalid event", "stream_error", "stream_failed")
				flusher.Flush()
			}
			if recorder != nil {
				recorder.RecordError(streamErr)
			}
			return
		}
		payload, err := protocolStageAnthropicBetaEventJSON(betaEvent, responseModel)
		if err != nil {
			if !wrote {
				ph.FailAttemptSetup(c, err)
			} else {
				protocolstream.MarshalAndSendErrorEvent(c, "Protocol Stage stream event conversion failed", "stream_error", "stream_failed")
				flusher.Flush()
			}
			return
		}
		if !wrote {
			setProtocolStageAnthropicSSEHeaders(c)
			wrote = true
		}
		switch betaEvent.Type {
		case "message_start":
			sawMessageStart = true
		case "message_stop":
			sawMessageStop = true
		case "content_block_delta":
			protocol.MarkFirstToken(c)
		}
		c.SSEvent(betaEvent.Type, string(payload))
		flusher.Flush()
	}

	if !wrote {
		setProtocolStageAnthropicSSEHeaders(c)
	}
	if sawMessageStart && !sawMessageStop {
		protocolstream.MarshalAndSendErrorEvent(c, "upstream stream ended before completion", "stream_error", "incomplete_stream")
		flusher.Flush()
	}
	result := stream.Result()
	if result.Usage != nil {
		ph.trackUsageWithTokenUsage(c, result.Usage, nil)
	}
}

func protocolStageAnthropicBetaEventJSON(event anthropic.BetaRawMessageStreamEventUnion, responseModel string) ([]byte, error) {
	raw := []byte(event.RawJSON())
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("marshal Anthropic Beta stream event: %w", err)
		}
	}
	if event.Type != "message_start" {
		return raw, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode Anthropic Beta message_start: %w", err)
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(object["message"], &message); err != nil {
		return nil, fmt.Errorf("decode Anthropic Beta message_start.message: %w", err)
	}
	model, err := json.Marshal(responseModel)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic Beta stream response model: %w", err)
	}
	message["model"] = model
	object["message"], err = json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic Beta message_start.message: %w", err)
	}
	payload, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic Beta message_start: %w", err)
	}
	return payload, nil
}

func setProtocolStageAnthropicSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}
