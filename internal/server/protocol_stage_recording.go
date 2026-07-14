package server

import (
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	internalobs "github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// protocolStageRequestRecording owns the new request-boundary recorder and its
// existing obs sink. It is intentionally separate from the legacy Gin-based
// ProtocolRecorder while the Stage path is canaried additively.
type protocolStageRequestRecording struct {
	recorder *requestrecord.Recorder
	sink     *internalobs.Sink
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
