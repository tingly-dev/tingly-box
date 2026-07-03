package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/pkg/obs"
)

func newModelRequestTestServer(t *testing.T) *WebHandler {
	t.Helper()
	cfg := &obs.MultiLoggerConfig{
		TextLogPath: "",
		JSONLogPath: "",
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP:         {MaxEntries: 100},
			obs.LogSourceModelRequest: {MaxEntries: 100},
			obs.LogSourceSmartRouting: {MaxEntries: 100},
		},
	}
	ml, err := obs.NewMultiLogger(cfg)
	assert.NoError(t, err)
	return NewWebHandler(WebDeps{MultiLogger: ml})
}

func emitHTTP(h *WebHandler, fields logrus.Fields) {
	h.deps.MultiLogger.GetLogrusLogger(obs.LogSourceHTTP).WithFields(fields).Info("http")
}

func emitModelStage(h *WebHandler, id, stage string, level logrus.Level) {
	h.deps.MultiLogger.GetLogrusLogger(obs.LogSourceModelRequest).
		WithFields(logrus.Fields{"request_id": id, "stage": stage}).Log(level, "stage")
}

func TestGetModelRequests_GroupsAndFilters(t *testing.T) {
	h := newModelRequestTestServer(t)

	// r1: a successful anthropic model request with a transform-stage warning.
	emitHTTP(h, logrus.Fields{
		"request_id": "r1", "status": 200, "method": "POST", "path": "/anthropic/v1/messages",
		"latency": 1500 * time.Millisecond, "scenario": "anthropic",
		"request_model": "claude-req", "routed_model": "claude-routed", "routed_provider": "prov-a",
	})
	emitModelStage(h, "r1", "transform", logrus.WarnLevel)

	// r2: a failed openai model request.
	emitHTTP(h, logrus.Fields{
		"request_id": "r2", "status": 500, "method": "POST", "path": "/openai/v1/chat/completions",
		"scenario": "openai", "request_model": "gpt-req",
	})

	// r3: a plain webui API call — has a request_id but no model semantics; must be excluded.
	emitHTTP(h, logrus.Fields{
		"request_id": "r3", "status": 200, "method": "GET", "path": "/api/v1/providers",
	})

	// Unfiltered: r1 and r2 only.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests", nil)
	h.GetModelRequests(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp ModelRequestsResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)

	byID := map[string]ModelRequestSummary{}
	for _, r := range resp.Requests {
		byID[r.RequestID] = r
	}
	if _, ok := byID["r3"]; ok {
		t.Fatalf("plain webui request r3 should be excluded from model requests")
	}

	r1 := byID["r1"]
	assert.Equal(t, "anthropic", r1.Scenario)
	assert.Equal(t, "claude-req", r1.RequestModel)
	assert.Equal(t, "claude-routed", r1.RoutedModel)
	assert.Equal(t, "prov-a", r1.Provider)
	assert.Equal(t, 200, r1.Status)
	assert.Equal(t, int64(1500), r1.LatencyMs)
	assert.Equal(t, 2, r1.EventCount) // http envelope + transform stage
	assert.False(t, r1.HasError)

	r2 := byID["r2"]
	assert.Equal(t, 500, r2.Status)
	assert.True(t, r2.HasError)

	// Filter by scenario.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests?scenario=anthropic", nil)
	h.GetModelRequests(c2)
	var filtered ModelRequestsResponse
	assert.NoError(t, json.Unmarshal(w2.Body.Bytes(), &filtered))
	assert.Equal(t, 1, filtered.Total)
	assert.Equal(t, "r1", filtered.Requests[0].RequestID)
}

func TestGetModelRequestDetail_TimelineOrdered(t *testing.T) {
	h := newModelRequestTestServer(t)

	emitModelStage(h, "rX", "inbound", logrus.InfoLevel)
	emitModelStage(h, "rX", "upstream", logrus.InfoLevel)
	emitHTTP(h, logrus.Fields{
		"request_id": "rX", "status": 200, "method": "POST", "path": "/anthropic/v1/messages",
		"scenario": "anthropic",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests/rX", nil)
	c.Params = gin.Params{{Key: "id", Value: "rX"}}
	h.GetModelRequestDetail(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var detail ModelRequestDetail
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, "rX", detail.RequestID)
	assert.Equal(t, 3, len(detail.Events))
	// Events are returned in chronological order.
	for i := 1; i < len(detail.Events); i++ {
		assert.False(t, detail.Events[i].Time.Before(detail.Events[i-1].Time))
	}

	// Unknown id -> 404.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests/nope", nil)
	c2.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.GetModelRequestDetail(c2)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}
