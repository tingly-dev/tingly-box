package server

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProtocolRecorder extends ScenarioRecorder for dual-stage recording
// It provides support for capturing both original and transformed requests,
// as well as provider and final responses (for protocol conversion scenarios)
type ProtocolRecorder struct {
	*ScenarioRecorder

	// Dual-stage request recording
	originalRequest    *obs.RecordRequest
	transformedRequest *obs.RecordRequest

	// Dual-stage response recording
	providerResponse *obs.RecordResponse
	finalResponse    *obs.RecordResponse

	// Transform chain information
	transformSteps []string

	// Recording metadata
	providerName string
	model        string
	mode         obs.RecordMode
}

// NewScenarioRecorderV2 creates a new ProtocolRecorder from an existing ScenarioRecorder
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

// SetOriginalRequest stores the pre-transform request
func (sr *ProtocolRecorder) SetOriginalRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.originalRequest = req
}

// SetTransformedRequest stores the post-transform request
func (sr *ProtocolRecorder) SetTransformedRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.transformedRequest = req
}

// SetProviderResponse stores the raw response from the provider
func (sr *ProtocolRecorder) SetProviderResponse(resp *obs.RecordResponse) {
	if sr == nil {
		return
	}
	sr.providerResponse = resp
}

// SetFinalResponse stores the final response to the client
func (sr *ProtocolRecorder) SetFinalResponse(resp *obs.RecordResponse) {
	if sr == nil {
		return
	}
	sr.finalResponse = resp
}

// SetTransformSteps records the transform steps that were applied
func (sr *ProtocolRecorder) SetTransformSteps(steps []string) {
	if sr == nil {
		return
	}
	sr.transformSteps = steps
}

// SetAssembledResponse stores the final assembled response for V2 recording.
// This mirrors ScenarioRecorder behavior but routes the final serialized body
// into ProtocolRecorder so RecordResponse() can emit RecordEntryV2.
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

// RecordResponse records a V2 protocol entry instead of falling back to the
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

// Record writes the V2 record entry to the sink
func (sr *ProtocolRecorder) Record() {
	if sr == nil || sr.sink == nil || sr.mode == "" {
		return
	}

	model := sr.model

	// Get model from original request if not provided
	if model == "" && sr.originalRequest != nil && sr.originalRequest.Body != nil {
		if m, ok := sr.originalRequest.Body["model"].(string); ok {
			model = m
		}
	}

	entry := &obs.RecordEntryV2{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Provider:       sr.providerName,
		Scenario:       sr.scenario,
		Model:          model,
		TransformSteps: sr.transformSteps,
		DurationMs:     time.Since(sr.startTime).Milliseconds(),
	}

	// Filter based on existing record mode
	switch sr.mode {
	case obs.RecordModeAll:
		// Record everything
		entry.OriginalRequest = sr.originalRequest
		entry.TransformedRequest = sr.transformedRequest
		entry.ProviderResponse = sr.providerResponse
		entry.FinalResponse = sr.finalResponse
	case obs.RecordModeRequestOnly:
		entry.TransformedRequest = sr.transformedRequest
	case obs.RecordModeRequestResponse:
		entry.TransformedRequest = sr.transformedRequest
		entry.FinalResponse = sr.finalResponse
	case obs.RecordModeStagedRequestResponse:
		entry.OriginalRequest = sr.originalRequest
		entry.TransformedRequest = sr.transformedRequest
		entry.FinalResponse = sr.finalResponse
	case obs.RecordModeScenario:
		// For scenario mode, record everything
		entry.OriginalRequest = sr.originalRequest
		entry.TransformedRequest = sr.transformedRequest
		entry.ProviderResponse = sr.providerResponse
		entry.FinalResponse = sr.finalResponse
	}

	sr.sink.RecordEntryV2(entry)
}

// RecordError records an error for V2 entries
func (sr *ProtocolRecorder) RecordError(err error) {
	if sr == nil || sr.sink == nil || sr.mode == "" {
		return
	}

	model := sr.model

	// Get model from original request if not provided
	if model == "" && sr.originalRequest != nil && sr.originalRequest.Body != nil {
		if m, ok := sr.originalRequest.Body["model"].(string); ok {
			model = m
		}
	}

	entry := &obs.RecordEntryV2{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Provider:   sr.providerName,
		Scenario:   sr.scenario,
		Model:      model,
		DurationMs: time.Since(sr.startTime).Milliseconds(),
		Error:      getErrorMessage(err),
	}

	// Filter based on existing record mode
	switch sr.mode {
	case obs.RecordModeAll, obs.RecordModeScenario, obs.RecordModeStagedRequestResponse:
		entry.OriginalRequest = sr.originalRequest
		entry.TransformedRequest = sr.transformedRequest
		entry.FinalResponse = sr.finalResponse
		if sr.mode == obs.RecordModeAll || sr.mode == obs.RecordModeScenario {
			entry.ProviderResponse = sr.providerResponse
		}
	case obs.RecordModeRequestOnly, obs.RecordModeRequestResponse:
		entry.TransformedRequest = sr.transformedRequest
		if sr.mode == obs.RecordModeRequestResponse {
			entry.FinalResponse = sr.finalResponse
		}
	}

	sr.sink.RecordEntryV2(entry)
}

// getErrorMessage safely extracts error message
func getErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetOrCreateScenarioRecorderV2 gets or creates a V2 scenario recorder for the given scenario
func (s *Server) GetOrCreateScenarioRecorderV2(c *gin.Context, scenario string, provider *typ.Provider, model string, mode obs.RecordMode) *ProtocolRecorder {
	// Use the existing scenario recorder if available
	if r, exists := c.Get("scenario_recorder"); exists {
		if rec, ok := r.(*ScenarioRecorder); ok {
			return NewScenarioRecorderV2(rec, provider, model, mode)
		}
	}

	// Create a new recorder if not available
	scenarioType := typ.RuleScenario(scenario)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	// Read the request body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Parse request body as JSON
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

	recorder := &ScenarioRecorder{
		sink:      sink,
		scenario:  scenario,
		req:       req,
		startTime: time.Now(),
		c:         c,
		bodyBytes: bodyBytes,
	}

	return NewScenarioRecorderV2(recorder, provider, model, mode)
}
