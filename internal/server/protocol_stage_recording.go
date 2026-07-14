package server

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	internalobs "github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const protocolStageOriginalInputKey = "protocol_stage_original_input"

// protocolStageRequestRecording owns the new request-boundary recorder and its
// existing obs sink. It is intentionally separate from the legacy Gin-based
// ProtocolRecorder while the Stage path is canaried additively.
type protocolStageRequestRecording struct {
	recorder *requestrecord.Recorder
	sink     *internalobs.Sink
}

func rememberProtocolStageOriginalInput(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	c.Set(protocolStageOriginalInputKey, json.RawMessage(append([]byte(nil), body...)))
}

func protocolStageOriginalInput(c *gin.Context, fallback any) any {
	if c == nil {
		return fallback
	}
	if input, ok := c.Get(protocolStageOriginalInputKey); ok {
		return input
	}
	return fallback
}

func (ph *ProtocolHandler) newProtocolStageRequestRecording(
	scenario typ.RuleScenario,
	inputProtocol protocol.APIType,
	input any,
	sessionID typ.SessionID,
	requestID string,
) *protocolStageRequestRecording {
	if ph == nil || !ph.deps.ProtocolStageEnabled || ph.deps.GetOrCreateScenarioSink == nil {
		return nil
	}
	sink := ph.deps.GetOrCreateScenarioSink(scenario)
	if sink == nil {
		return nil
	}
	sessionShort, _ := internalobs.SessionShort(sessionID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	recorder, err := requestrecord.New(requestrecord.Config{
		Enabled:       true,
		RequestID:     requestID,
		SessionID:     sessionShort,
		Scenario:      string(scenario),
		InputProtocol: inputProtocol,
		Input:         input,
	})
	if err != nil {
		logrus.Debugf("obs: failed to build Protocol Stage RequestRecord: %v", err)
		return nil
	}
	return &protocolStageRequestRecording{recorder: recorder, sink: sink}
}

func (r *protocolStageRequestRecording) finish(requestErr error) {
	if r == nil || r.recorder == nil || r.sink == nil {
		return
	}
	completed, first := r.recorder.Finish(requestErr)
	if !first || completed == nil {
		return
	}
	r.sink.EmitRequestRecord(completed)
}

func stageRecordingRecorder(recording *protocolStageRequestRecording) *requestrecord.Recorder {
	if recording == nil {
		return nil
	}
	return recording.recorder
}

func captureProtocolStageFinalResponse(
	ctx context.Context,
	recorder *requestrecord.Recorder,
	api protocol.APIType,
	response any,
) {
	if recorder == nil {
		return
	}
	if err := recorder.SetFinalResponse(api, response); err != nil {
		logrus.WithContext(ctx).WithError(err).Debug("Protocol Stage RequestRecord final response capture failed")
	}
}

type protocolStageFinalStreamCapture struct {
	recorder  *requestrecord.Recorder
	protocol  protocol.APIType
	assembler assembler.StreamAssembler
}

func newProtocolStageFinalStreamCapture(
	ctx context.Context,
	recorder *requestrecord.Recorder,
	api protocol.APIType,
) *protocolStageFinalStreamCapture {
	if recorder == nil {
		return nil
	}
	streamAssembler, err := assembler.NewStreamAssembler(api)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).Debug("Protocol Stage RequestRecord final stream assembler unavailable")
		return nil
	}
	return &protocolStageFinalStreamCapture{
		recorder:  recorder,
		protocol:  api,
		assembler: streamAssembler,
	}
}

func (c *protocolStageFinalStreamCapture) add(ctx context.Context, event any) {
	if c == nil || c.assembler == nil {
		return
	}
	if err := c.assembler.Add(event); err != nil {
		logrus.WithContext(ctx).WithError(err).Debug("Protocol Stage RequestRecord final stream event capture failed")
		c.assembler = nil
	}
}

func (c *protocolStageFinalStreamCapture) finish(ctx context.Context) {
	if c == nil || c.assembler == nil || c.recorder == nil {
		return
	}
	response, err := c.assembler.Finish()
	if err != nil {
		logrus.WithContext(ctx).WithError(err).Debug("Protocol Stage RequestRecord final stream assembly failed")
		return
	}
	captureProtocolStageFinalResponse(ctx, c.recorder, c.protocol, response)
}
