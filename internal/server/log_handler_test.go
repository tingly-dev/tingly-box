package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestLogServer creates a test server with logging middleware
func setupTestLogServer() (*Server, *middleware.MultiModeMemoryLogMiddleware) {
	config := &obs.MultiLoggerConfig{
		TextLogPath: "",
		JSONLogPath: "",
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP: {MaxEntries: 100},
		},
	}
	multiLogger, err := obs.NewMultiLogger(config)
	if err != nil {
		panic(err)
	}
	memoryLogMW := middleware.NewMultiModeMemoryLogMiddleware(multiLogger)

	return &Server{
		memoryLogMW: memoryLogMW,
	}, memoryLogMW
}

func TestGetRequestBody_Success(t *testing.T) {
	server, memoryLogMW := setupTestLogServer()

	// Store a request body
	store := memoryLogMW.GetRequestBodyStore()
	testBody := `{"test": "data", "value": 123}`
	bodyID := store.Store("POST", "/v1/chat/completions", testBody, 1024)

	// Create gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: bodyID}}

	// Call handler
	server.GetRequestBody(c)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp RequestBodyResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)

	assert.Equal(t, bodyID, resp.ID)
	assert.Equal(t, "POST", resp.Method)
	assert.Equal(t, "/v1/chat/completions", resp.Path)
	assert.Equal(t, testBody, resp.Body)
	assert.False(t, resp.Truncated)
}

func TestGetRequestBody_NotFound(t *testing.T) {
	server, _ := setupTestLogServer()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "nonexistent_id"}}

	server.GetRequestBody(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Contains(t, resp["error"], "not found")
}

func TestGetRequestBody_MissingID(t *testing.T) {
	server, _ := setupTestLogServer()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// No id parameter set

	server.GetRequestBody(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Contains(t, resp["error"], "Missing request body ID")
}

func TestGetRequestBody_TruncatedBody(t *testing.T) {
	server, memoryLogMW := setupTestLogServer()

	// Store a body that will be truncated
	store := memoryLogMW.GetRequestBodyStore()
	longBody := string(make([]byte, 2048))                 // 2KB body
	bodyID := store.Store("POST", "/test", longBody, 1024) // Max 1KB

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: bodyID}}

	server.GetRequestBody(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RequestBodyResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)

	assert.True(t, resp.Truncated)
	assert.LessOrEqual(t, len(resp.Body), 1024)
}

func TestClearRequestBodies_Success(t *testing.T) {
	server, memoryLogMW := setupTestLogServer()

	// Store some bodies
	store := memoryLogMW.GetRequestBodyStore()
	store.Store("POST", "/test1", "body1", 1024)
	store.Store("POST", "/test2", "body2", 1024)

	assert.Equal(t, 2, store.Size())

	// Call handler
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	server.ClearRequestBodies(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, store.Size(), "Store should be empty after clear")

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "Request bodies cleared successfully", resp["message"])
}

func TestGetRequestBodyStats_Success(t *testing.T) {
	server, memoryLogMW := setupTestLogServer()

	// Store some bodies
	store := memoryLogMW.GetRequestBodyStore()
	store.Store("POST", "/test1", "body1", 1024)
	store.Store("POST", "/test2", "body2", 1024)
	store.Store("POST", "/test3", "body3", 1024)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	server.GetRequestBodyStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)

	assert.Equal(t, float64(3), resp["total"])
	assert.Equal(t, float64(500), resp["capacity"])
}

func TestGetRequestBodyStats_Empty(t *testing.T) {
	server, _ := setupTestLogServer()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	server.GetRequestBodyStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)

	assert.Equal(t, float64(0), resp["total"])
}

func TestGetRequestBody_NilMiddleware(t *testing.T) {
	server := &Server{
		memoryLogMW: nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "id", Value: "test_id"}}

	server.GetRequestBody(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Contains(t, resp["error"], "not available")
}

func TestClearRequestBodies_NilMiddleware(t *testing.T) {
	server := &Server{
		memoryLogMW: nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	server.ClearRequestBodies(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGetRequestBodyStats_NilMiddleware(t *testing.T) {
	server := &Server{
		memoryLogMW: nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	server.GetRequestBodyStats(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGetLogs_IntegrationWithRequestBody(t *testing.T) {
	server, memoryLogMW := setupTestLogServer()

	// Create a test engine with middleware
	engine := gin.New()
	engine.Use(memoryLogMW.Middleware())
	engine.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// Make a POST request with body
	testBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	req.Body = io.NopCloser(bytes.NewBufferString(testBody))
	engine.ServeHTTP(w, req)

	// Get logs which should include body_ref
	logW := httptest.NewRecorder()
	logC, _ := gin.CreateTestContext(logW)
	logC.Request = httptest.NewRequest("GET", "/api/logs?limit=1", nil)
	server.GetLogs(logC)

	assert.Equal(t, http.StatusOK, logW.Code)

	var logsResp LogsResponse
	err := json.Unmarshal(logW.Body.Bytes(), &logsResp)
	assert.NoError(t, err)
	assert.Greater(t, len(logsResp.Logs), 0, "Expected at least one log entry")

	// Check if body_ref is present in the log entry
	lastLog := logsResp.Logs[len(logsResp.Logs)-1]
	if bodyRef, ok := lastLog.Fields["body_ref"].(string); ok {
		assert.NotEmpty(t, bodyRef, "Expected body_ref to be non-empty")

		// Now verify we can retrieve the body using the ref
		bodyW := httptest.NewRecorder()
		bodyC, _ := gin.CreateTestContext(bodyW)
		bodyC.Params = []gin.Param{{Key: "id", Value: bodyRef}}
		server.GetRequestBody(bodyC)

		assert.Equal(t, http.StatusOK, bodyW.Code)

		var bodyResp RequestBodyResponse
		err = json.Unmarshal(bodyW.Body.Bytes(), &bodyResp)
		assert.NoError(t, err)
		assert.Equal(t, "POST", bodyResp.Method)
		assert.Equal(t, "/test", bodyResp.Path)
		assert.Equal(t, testBody, bodyResp.Body)
	}
}

func TestConvertLogrusEntry_WithBodyRef(t *testing.T) {
	entry := &logrus.Entry{
		Message: "Test message",
		Level:   logrus.InfoLevel,
		Data: map[string]interface{}{
			"type":     "http_request",
			"status":   200,
			"method":   "POST",
			"path":     "/v1/chat/completions",
			"body_ref": "req_12345",
		},
	}

	result := convertLogrusEntry(entry)

	assert.Equal(t, "Test message", result.Message)
	assert.Equal(t, "info", result.Level)
	assert.NotNil(t, result.Fields)
	assert.Equal(t, 200, result.Fields["status"])
	assert.Equal(t, "POST", result.Fields["method"])
	assert.Equal(t, "/v1/chat/completions", result.Fields["path"])
	assert.Equal(t, "req_12345", result.Fields["body_ref"])
}
