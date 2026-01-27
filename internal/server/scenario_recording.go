package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// RecordScenarioRequest records the scenario-level request (client -> tingly-box)
// This captures the original request from the client before any transformation
func (s *Server) RecordScenarioRequest(c *gin.Context, scenario string) *ScenarioRecorder {
	if s.scenarioRecordSink == nil || !s.scenarioRecordSink.IsEnabled() {
		return nil
	}

	// Read and restore the request body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.Debugf("Failed to read request body for scenario recording: %v", err)
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Parse request body as JSON
	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			logrus.Debugf("Failed to parse request body as JSON: %v", err)
			// Keep raw body as string if JSON parsing fails
			bodyJSON = map[string]interface{}{"raw": string(bodyBytes)}
		}
	}

	req := &obs.RecordRequest{
		Method:  c.Request.Method,
		URL:     c.Request.URL.String(),
		Headers: headerToMap(c.Request.Header),
		Body:    bodyJSON,
	}

	return &ScenarioRecorder{
		sink:      s.scenarioRecordSink,
		scenario:  scenario,
		req:       req,
		startTime: time.Now(),
		c:         c,
		bodyBytes: bodyBytes,
	}
}

// ScenarioRecorder captures scenario-level request/response recording
type ScenarioRecorder struct {
	sink      *obs.Sink
	scenario  string
	req       *obs.RecordRequest
	startTime time.Time
	c         *gin.Context
	bodyBytes []byte
}

// RecordResponse records the scenario-level response (tingly-box -> client)
// This captures the response sent back to the client
func (sr *ScenarioRecorder) RecordResponse() {
	if sr == nil || sr.sink == nil {
		return
	}

	// Get response info from the context
	statusCode := sr.c.Writer.Status()
	headers := headerToMap(sr.c.Writer.Header())

	// Try to get response body if it was captured
	var bodyJSON map[string]interface{}
	if responseBody, exists := sr.c.Get("response_body"); exists {
		if bytes, ok := responseBody.([]byte); ok {
			if err := json.Unmarshal(bytes, &bodyJSON); err == nil {
				bodyJSON = map[string]interface{}{"raw": string(bytes)}
			}
		}
	}

	resp := &obs.RecordResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       bodyJSON,
	}

	// Extract model from request if available
	model := ""
	if sr.req.Body != nil {
		if m, ok := sr.req.Body["model"].(string); ok {
			model = m
		}
	}

	// Record with scenario-based file naming
	duration := time.Since(sr.startTime)
	sr.sink.RecordWithScenario("tingly-box", model, sr.scenario, sr.req, resp, duration, nil)
}

// RecordError records an error for the scenario-level request
func (sr *ScenarioRecorder) RecordError(err error) {
	if sr == nil || sr.sink == nil {
		return
	}

	resp := &obs.RecordResponse{
		StatusCode: sr.c.Writer.Status(),
		Headers:    headerToMap(sr.c.Writer.Header()),
	}

	// Extract model from request if available
	model := ""
	if sr.req.Body != nil {
		if m, ok := sr.req.Body["model"].(string); ok {
			model = m
		}
	}

	// Record with error
	duration := time.Since(sr.startTime)
	sr.sink.RecordWithScenario("tingly-box", model, sr.scenario, sr.req, resp, duration, err)
}

// headerToMap converts http.Header to map[string]string
func headerToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
