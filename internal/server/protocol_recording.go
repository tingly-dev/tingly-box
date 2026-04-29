package server

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProtocolRecorder extends ScenarioRecorder for dual-stage recording.
// It captures original and transformed requests plus provider and final
// responses for protocol-conversion scenarios.
type ProtocolRecorder struct {
	*ScenarioRecorder

	originalRequest    *obs.RecordRequest
	transformedRequest *obs.RecordRequest
	providerResponse   *obs.RecordResponse
	finalResponse      *obs.RecordResponse

	transformSteps []string

	providerName string
	model        string
	mode         obs.RecordMode
}

// NewScenarioRecorderV2 creates a ProtocolRecorder from an existing ScenarioRecorder.
func NewScenarioRecorderV2(recorder *ScenarioRecorder, provider *typ.Provider, model string, mode obs.RecordMode) *ProtocolRecorder {
	if recorder == nil {
		return nil
	}
	providerName := ""
	if provider != nil {
		providerName = provider.Name
	}
	return &ProtocolRecorder{
		ScenarioRecorder: recorder,
		providerName:     providerName,
		model:            model,
		mode:             mode,
	}
}

// SetOriginalRequest stores the pre-transform request.
func (sr *ProtocolRecorder) SetOriginalRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.originalRequest = req
}

// SetTransformedRequest stores the post-transform request.
func (sr *ProtocolRecorder) SetTransformedRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.transformedRequest = req
}

// SetProviderResponse stores the raw provider response.
func (sr *ProtocolRecorder) SetProviderResponse(resp *obs.RecordResponse) {
	if sr == nil {
		return
	}
	sr.providerResponse = resp
}

// SetFinalResponse stores the final response sent to the client.
func (sr *ProtocolRecorder) SetFinalResponse(resp *obs.RecordResponse) {
	if sr == nil {
		return
	}
	sr.finalResponse = resp
}

// SetTransformSteps records which transforms were applied.
func (sr *ProtocolRecorder) SetTransformSteps(steps []string) {
	if sr == nil {
		return
	}
	sr.transformSteps = steps
}

// SetAssembledResponse stores the final assembled response. Routes the final
// serialized body into ProtocolRecorder so Record() can emit a *obs.Record.
func (sr *ProtocolRecorder) SetAssembledResponse(response any) {
	if sr == nil {
		return
	}

	var responseMap map[string]interface{}
	switch v := response.(type) {
	case map[string]interface{}:
		responseMap = v
	case []byte:
		if err := json.Unmarshal(v, &responseMap); err != nil {
			logrus.Debugf("ProtocolRecorder: failed to unmarshal response bytes: %v", err)
			return
		}
	default:
		data, err := json.Marshal(response)
		if err != nil {
			logrus.Debugf("ProtocolRecorder: failed to marshal response: %v", err)
			return
		}
		if err := json.Unmarshal(data, &responseMap); err != nil {
			logrus.Debugf("ProtocolRecorder: failed to unmarshal marshaled response: %v", err)
			return
		}
	}

	statusCode := 200
	headers := map[string]string{}
	if sr.c != nil {
		statusCode = sr.c.Writer.Status()
		headers = headerToMap(sr.c.Writer.Header())
	}

	sr.SetFinalResponse(&obs.RecordResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       responseMap,
	})
}

// RecordResponse records a protocol entry instead of falling back to the
// embedded ScenarioRecorder's old-format RecordWithScenario path.
func (sr *ProtocolRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	if provider != nil {
		sr.providerName = provider.Name
	}
	if model != "" {
		sr.model = model
	}
	sr.Record()
}

// Record emits a *obs.Record to the sink's async pipeline.
func (sr *ProtocolRecorder) Record() {
	if sr == nil || sr.sink == nil || sr.mode == "" {
		return
	}

	model := sr.model
	if model == "" && sr.originalRequest != nil && sr.originalRequest.Body != nil {
		if m, ok := sr.originalRequest.Body["model"].(string); ok {
			model = m
		}
	}

	r := &obs.Record{
		Timestamp:  time.Now().UTC(),
		RequestID:  uuid.New().String(),
		SessionID:  sr.sessionShort,
		SessionSrc: sr.sessionSrc,
		Provider:   sr.providerName,
		Scenario:   sr.scenario,
		Model:      model,
		Steps:      sr.transformSteps,
		Duration:   time.Since(sr.startTime),
	}

	switch sr.mode {
	case obs.RecordModeAll, obs.RecordModeScenario:
		r.OriginalRequest = sr.originalRequest
		r.TransformedRequest = sr.transformedRequest
		r.ProviderResponse = sr.providerResponse
		r.FinalResponse = sr.finalResponse
	case obs.RecordModeRequestOnly:
		r.TransformedRequest = sr.transformedRequest
	case obs.RecordModeRequestResponse:
		r.TransformedRequest = sr.transformedRequest
		r.FinalResponse = sr.finalResponse
	case obs.RecordModeStagedRequestResponse:
		r.OriginalRequest = sr.originalRequest
		r.TransformedRequest = sr.transformedRequest
		r.FinalResponse = sr.finalResponse
	}

	sr.sink.Emit(r)
}

// RecordError emits an error record.
func (sr *ProtocolRecorder) RecordError(err error) {
	if sr == nil || sr.sink == nil || sr.mode == "" {
		return
	}

	model := sr.model
	if model == "" && sr.originalRequest != nil && sr.originalRequest.Body != nil {
		if m, ok := sr.originalRequest.Body["model"].(string); ok {
			model = m
		}
	}

	r := &obs.Record{
		Timestamp:  time.Now().UTC(),
		RequestID:  uuid.New().String(),
		SessionID:  sr.sessionShort,
		SessionSrc: sr.sessionSrc,
		Provider:   sr.providerName,
		Scenario:   sr.scenario,
		Model:      model,
		Duration:   time.Since(sr.startTime),
		Err:        getErrorMessage(err),
	}

	switch sr.mode {
	case obs.RecordModeAll, obs.RecordModeScenario, obs.RecordModeStagedRequestResponse:
		r.OriginalRequest = sr.originalRequest
		r.TransformedRequest = sr.transformedRequest
		r.FinalResponse = sr.finalResponse
		if sr.mode == obs.RecordModeAll || sr.mode == obs.RecordModeScenario {
			r.ProviderResponse = sr.providerResponse
		}
	case obs.RecordModeRequestOnly, obs.RecordModeRequestResponse:
		r.TransformedRequest = sr.transformedRequest
		if sr.mode == obs.RecordModeRequestResponse {
			r.FinalResponse = sr.finalResponse
		}
	}

	sr.sink.Emit(r)
}

// getErrorMessage safely extracts the error message string.
func getErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetOrCreateScenarioRecorderV2 returns a ProtocolRecorder for the given scenario,
// reusing any existing ScenarioRecorder stored in the gin context.
func (s *Server) GetOrCreateScenarioRecorderV2(c *gin.Context, scenario string, provider *typ.Provider, model string, mode obs.RecordMode) *ProtocolRecorder {
	if r, exists := c.Get("scenario_recorder"); exists {
		if rec, ok := r.(*ScenarioRecorder); ok {
			return NewScenarioRecorderV2(rec, provider, model, mode)
		}
	}

	scenarioType := typ.RuleScenario(scenario)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			bodyJSON = map[string]interface{}{"raw": string(bodyBytes)}
		}
	}

	req := &obs.RecordRequest{
		Method:  c.Request.Method,
		URL:     c.Request.URL.String(),
		Headers: headerToMap(c.Request.Header),
		Body:    bodyJSON,
	}

	sid := typ.GetSessionID(c.Request.Context())
	short, src := obs.SessionShort(sid)

	recorder := &ScenarioRecorder{
		sink:         sink,
		scenario:     scenario,
		req:          req,
		startTime:    time.Now(),
		c:            c,
		bodyBytes:    bodyBytes,
		sessionShort: short,
		sessionSrc:   src,
	}

	return NewScenarioRecorderV2(recorder, provider, model, mode)
}
