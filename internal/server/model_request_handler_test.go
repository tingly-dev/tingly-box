package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
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

func TestRequestHistorySurvivesEvictionAndRestart(t *testing.T) {
	cfg := obs.DefaultMultiLoggerConfig(t.TempDir())
	cfg.MemorySinkConfig = map[obs.LogSource]obs.MemorySinkConfig{
		obs.LogSourceHTTP: {MaxEntries: 1}, obs.LogSourceModelRequest: {MaxEntries: 1},
	}
	ml, err := obs.NewMultiLogger(cfg)
	require.NoError(t, err)
	h := NewWebHandler(WebDeps{MultiLogger: ml})
	emitModelStage(h, "original", "upstream", logrus.ErrorLevel)
	emitHTTP(h, logrus.Fields{"request_id": "original", "scenario": "openai", "status": 502, "latency": 250 * time.Millisecond, "error": "upstream unavailable"})
	emitModelStage(h, "later", "upstream", logrus.InfoLevel)
	emitHTTP(h, logrus.Fields{"request_id": "later", "scenario": "openai", "status": 200})
	groups, err := h.collectRequestGroups()
	require.NoError(t, err)
	require.Len(t, groups["original"].events, 2)
	require.Len(t, groups["later"].events, 2) // persisted + memory copies dedupe
	require.NoError(t, ml.Close())
	ml, err = obs.NewMultiLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	h = NewWebHandler(WebDeps{MultiLogger: ml})
	groups, err = h.collectRequestGroups()
	require.NoError(t, err)
	require.Equal(t, 502, groups["original"].summary.Status)
	require.Equal(t, int64(250), groups["original"].summary.LatencyMs)
	require.True(t, groups["original"].summary.HasError)
	require.Len(t, groups["original"].events, 2)
	// A read failure must not masquerade as an empty request history.
	cfg.JSONLogPath = filepath.Dir(cfg.JSONLogPath)
	ml2, err := obs.NewMultiLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml2.Close() })
	_, err = NewWebHandler(WebDeps{MultiLogger: ml2}).collectRequestGroups()
	require.Error(t, err)
}

func TestRequestHistoryWindowExcludesUnrelatedLogs(t *testing.T) {
	cfg := obs.DefaultMultiLoggerConfig(t.TempDir())
	ml, err := obs.NewMultiLogger(cfg)
	require.NoError(t, err)
	h := NewWebHandler(WebDeps{MultiLogger: ml})
	emitModelStage(h, "original", "upstream", logrus.ErrorLevel)
	emitHTTP(h, logrus.Fields{"request_id": "original", "scenario": "openai", "status": 500})
	for i := 0; i <= recentRequestEventLimit; i++ {
		emitHTTP(h, logrus.Fields{"request_id": "poll", "path": "/api/v1/requests", "status": 200})
		ml.GetLogrusLogger(obs.LogSourceSystem).Info("background heartbeat")
	}
	require.NoError(t, ml.Close())
	ml, err = obs.NewMultiLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	groups, err := NewWebHandler(WebDeps{MultiLogger: ml}).collectRequestGroups()
	require.NoError(t, err)
	require.Contains(t, groups, "original")
	require.Len(t, groups["original"].events, 2)
	require.NotContains(t, groups, "poll")
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

// TestGetModelRequests_FailoverSummary pins the list-level failover
// visibility: stage=failover_retry events fold into FailoverHops and a
// readable FailoverPath, so the Requests view can answer "did this request
// fail over, and to where" without opening the event timeline.
func TestGetModelRequests_FailoverSummary(t *testing.T) {
	h := newModelRequestTestServer(t)

	emitHTTP(h, logrus.Fields{
		"request_id": "rf", "status": 200, "method": "POST", "path": "/anthropic/v1/messages",
		"scenario": "anthropic", "request_model": "claude-req", "routed_provider": "prov-c",
	})
	ml := h.deps.MultiLogger.GetLogrusLogger(obs.LogSourceModelRequest)
	ml.WithFields(logrus.Fields{
		"request_id": "rf", "stage": "failover_retry",
		"from_service": "prov-a/model-a", "to_service": "prov-b/model-b",
	}).Warn("retry 1")
	ml.WithFields(logrus.Fields{
		"request_id": "rf", "stage": "failover_retry",
		"from_service": "prov-b/model-b", "to_service": "prov-c/model-c",
	}).Warn("retry 2")
	ml.WithFields(logrus.Fields{
		"request_id": "rf", "stage": "failover_success",
	}).Info("succeeded")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests", nil)
	h.GetModelRequests(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp ModelRequestsResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	var rf *ModelRequestSummary
	for i := range resp.Requests {
		if resp.Requests[i].RequestID == "rf" {
			rf = &resp.Requests[i]
		}
	}
	if rf == nil {
		t.Fatal("request rf missing from response")
	}
	assert.Equal(t, 2, rf.FailoverHops)
	assert.Equal(t, "prov-a/model-a → prov-b/model-b → prov-c/model-c", rf.FailoverPath)

	// A request without failover events reports zero hops and no path.
	emitHTTP(h, logrus.Fields{
		"request_id": "rn", "status": 200, "method": "POST", "path": "/anthropic/v1/messages",
		"scenario": "anthropic", "request_model": "claude-req",
	})
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/requests", nil)
	h.GetModelRequests(c)
	resp = ModelRequestsResponse{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, r := range resp.Requests {
		if r.RequestID == "rn" {
			assert.Equal(t, 0, r.FailoverHops)
			assert.Equal(t, "", r.FailoverPath)
		}
	}
}
