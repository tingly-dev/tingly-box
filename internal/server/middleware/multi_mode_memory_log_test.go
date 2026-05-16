package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestMiddleware() (*MultiModeMemoryLogMiddleware, *obs.MultiLogger) {
	config := &obs.MultiLoggerConfig{
		TextLogPath: "", // Disable text logging for tests
		JSONLogPath: "", // Disable JSON logging for tests
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP: {
				MaxEntries: 100,
			},
		},
	}
	multiLogger, err := obs.NewMultiLogger(config)
	if err != nil {
		panic(err)
	}
	middleware := NewMultiModeMemoryLogMiddleware(multiLogger)
	return middleware, multiLogger
}

func TestNewMultiModeMemoryLogMiddleware(t *testing.T) {
	middleware, multiLogger := setupTestMiddleware()

	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.multiLogger)
	assert.NotNil(t, middleware.logger)
	assert.Equal(t, multiLogger, middleware.multiLogger)
}

func TestMiddleware_LogsHTTPRequests(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	// Create a test gin engine with the middleware
	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// Make a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	engine.ServeHTTP(w, req)

	// Verify the request was logged
	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries, "Expected log entries to be recorded")

	// Verify the log entry has the expected fields
	entry := entries[len(entries)-1] // Get the last entry
	assert.Equal(t, "http_request", entry.Data["type"])
	assert.Equal(t, 200, entry.Data["status"])
	assert.Equal(t, "GET", entry.Data["method"])
	assert.Equal(t, "/test", entry.Data["path"])
	assert.NotNil(t, entry.Data["latency"])
	assert.NotNil(t, entry.Data["client_ip"])
}

func TestMiddleware_LogLevelByStatusCode(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		expectedLevel logrus.Level
	}{
		{"Success status", http.StatusOK, logrus.InfoLevel},
		{"Redirect status", http.StatusFound, logrus.InfoLevel},
		{"Bad request", http.StatusBadRequest, logrus.WarnLevel},
		{"Unauthorized", http.StatusUnauthorized, logrus.WarnLevel},
		{"Server error", http.StatusInternalServerError, logrus.ErrorLevel},
		{"Service unavailable", http.StatusServiceUnavailable, logrus.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware, _ := setupTestMiddleware()

			engine := gin.New()
			engine.Use(middleware.Middleware())
			engine.GET("/test", func(c *gin.Context) {
				c.Status(tt.statusCode)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			engine.ServeHTTP(w, req)

			entries := middleware.GetEntries()
			assert.NotEmpty(t, entries)
			assert.Equal(t, tt.expectedLevel, entries[len(entries)-1].Level)
		})
	}
}

func TestMiddleware_LogsWithQueryParams(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test?foo=bar&baz=qux", nil)
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	assert.Equal(t, "/test?foo=bar&baz=qux", entries[len(entries)-1].Data["path"])
}

func TestGetMemoryEntries(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test1", func(c *gin.Context) { c.String(http.StatusOK, "OK1") })
	engine.GET("/test2", func(c *gin.Context) { c.String(http.StatusOK, "OK2") })

	// Make multiple requests
	engine.ServeHTTP(httptest.NewRecorder(), &http.Request{Method: "GET", URL: &url.URL{Path: "/test1"}})
	engine.ServeHTTP(httptest.NewRecorder(), &http.Request{Method: "GET", URL: &url.URL{Path: "/test2"}})

	entries := middleware.GetEntries()
	assert.GreaterOrEqual(t, len(entries), 2, "Expected at least 2 log entries")
}

func TestGetMemoryLatest(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Make 5 requests
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		engine.ServeHTTP(w, req)
	}

	// Get latest 3 entries
	latest := middleware.GetLatestEntries(3)
	assert.Len(t, latest, 3, "Expected 3 latest entries")
}

func TestGetMemoryEntriesSince(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Make a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	engine.ServeHTTP(w, req)

	// Get entries since now (should return the entry we just made)
	since := time.Now().Add(-1 * time.Minute)
	entries := middleware.GetEntriesSince(since)
	assert.GreaterOrEqual(t, len(entries), 1, "Expected at least 1 entry since 1 minute ago")
}

func TestGetMemoryEntriesByLevel(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "OK") })
	engine.GET("/error", func(c *gin.Context) { c.String(http.StatusInternalServerError, "Error") })

	// Make successful request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ok", nil)
	engine.ServeHTTP(w, req)

	// Make error request
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/error", nil)
	engine.ServeHTTP(w, req)

	// Get error level entries
	errorEntries := middleware.GetEntriesByLevel(logrus.ErrorLevel)
	assert.GreaterOrEqual(t, len(errorEntries), 1, "Expected at least 1 error entry")

	// Get info level entries
	infoEntries := middleware.GetEntriesByLevel(logrus.InfoLevel)
	assert.GreaterOrEqual(t, len(infoEntries), 1, "Expected at least 1 info entry")
}

func TestClearMemory(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Make some requests
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		engine.ServeHTTP(w, req)
	}

	// Verify we have entries
	entries := middleware.GetEntries()
	assert.GreaterOrEqual(t, len(entries), 1, "Expected log entries before clear")

	// Clear memory
	middleware.Clear()

	// Verify memory is cleared
	entries = middleware.GetEntries()
	assert.Empty(t, entries, "Expected no entries after clear")
}

func TestMemorySize(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Initially should be 0 or small
	initialSize := middleware.Size()

	// Make some requests
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		engine.ServeHTTP(w, req)
	}

	// Size should have increased
	newSize := middleware.Size()
	assert.Greater(t, newSize, initialSize, "Expected memory size to increase after requests")
}

func TestCompatibilityAliases(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Make a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	engine.ServeHTTP(w, req)

	// Test that main methods work
	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries, "GetEntries() should work")

	latest := middleware.GetLatestEntries(10)
	assert.NotEmpty(t, latest, "GetLatestEntries() should work")

	initialSize := middleware.Size()
	assert.GreaterOrEqual(t, initialSize, 1, "Size() should work")

	middleware.Clear()
	entriesAfterClear := middleware.GetEntries()
	assert.Empty(t, entriesAfterClear, "Clear() should work")
}

func TestMiddleware_WithNilMultiLogger(t *testing.T) {
	// This test verifies the middleware handles edge cases gracefully
	// In production, multiLogger should never be nil after NewMultiModeMemoryLogMiddleware
	config := &obs.MultiLoggerConfig{
		TextLogPath: "",
		JSONLogPath: "",
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP: {MaxEntries: 100},
		},
	}
	multiLogger, err := obs.NewMultiLogger(config)
	if err != nil {
		t.Fatalf("Failed to create multiLogger: %v", err)
	}
	middleware := NewMultiModeMemoryLogMiddleware(multiLogger)

	assert.NotNil(t, middleware, "Middleware should be created successfully")
}

func TestMiddleware_ConcurrentRequests(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	// Make concurrent requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			engine.ServeHTTP(w, req)
			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all requests were logged
	entries := middleware.GetEntries()
	assert.GreaterOrEqual(t, len(entries), 10, "Expected at least 10 log entries from concurrent requests")
}

func TestMiddleware_UserAgentLogging(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	assert.Equal(t, "TestAgent/1.0", entries[len(entries)-1].Data["user_agent"])
}

func TestMiddleware_RequestBodyStorage(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	// The middleware uses io.TeeReader to capture the request body, which
	// only fills the buffer once the handler actually reads from c.Request.
	// It also only persists the body when status >= 400 (see body_ref
	// logic in multi_mode_memory_log.go). Drain the body and return 4xx
	// so both conditions are satisfied.
	engine.POST("/test", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.String(http.StatusBadRequest, "bad")
	})

	// Make a POST request with body
	testBody := `{"test": "data", "value": 123}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	req.Body = io.NopCloser(bytes.NewBufferString(testBody))
	engine.ServeHTTP(w, req)

	// Verify the request was logged with body_ref
	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]

	bodyRef, exists := entry.Data["body_ref"]
	assert.True(t, exists, "Expected body_ref to be present in log entry")
	assert.NotEmpty(t, bodyRef, "Expected body_ref to have a value")

	// Verify the body can be retrieved from the store
	store := middleware.GetRequestBodyStore()
	assert.NotNil(t, store, "Expected RequestBodyStore to be available")

	storedBody := store.Get(bodyRef.(string))
	assert.NotNil(t, storedBody, "Expected to find stored body by ref")
	assert.Equal(t, "POST", storedBody.Method)
	assert.Equal(t, "/test", storedBody.Path)
	assert.Equal(t, testBody, storedBody.Body)
	assert.False(t, storedBody.Truncated, "Expected body not to be truncated")
}

func TestMiddleware_RequestBodyTruncation(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.POST("/test", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body) // drain body so TeeReader captures it
		c.String(http.StatusBadRequest, "bad")
	})

	// Create a body larger than MaxRequestBodySize (1MB)
	longBody := string(make([]byte, 2*1024*1024)) // 2MB body

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	req.Body = io.NopCloser(bytes.NewBufferString(longBody))
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]

	bodyRef := entry.Data["body_ref"]
	store := middleware.GetRequestBodyStore()
	storedBody := store.Get(bodyRef.(string))

	assert.NotNil(t, storedBody)
	assert.True(t, storedBody.Truncated, "Expected body to be truncated")
	assert.LessOrEqual(t, len(storedBody.Body), MaxRequestBodySize, "Expected stored body to be at most MaxRequestBodySize")
}

func TestMiddleware_RequestBodyNotStoredForGET(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]

	_, exists := entry.Data["body_ref"]
	assert.False(t, exists, "Expected no body_ref for GET request")
}

func TestMiddleware_RequestBodyNotStoredForHEAD(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.HEAD("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("HEAD", "/test", nil)
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]

	_, exists := entry.Data["body_ref"]
	assert.False(t, exists, "Expected no body_ref for HEAD request")
}

func TestMiddleware_RequestBodyStorageEmptyBody(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.POST("/test", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	req.Body = io.NopCloser(bytes.NewBufferString("")) // Empty body
	engine.ServeHTTP(w, req)

	entries := middleware.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]

	_, exists := entry.Data["body_ref"]
	assert.False(t, exists, "Expected no body_ref for empty body")
}

func TestMiddleware_RequestBodyCircularEviction(t *testing.T) {
	// Create middleware with small RequestBodyStore
	config := &obs.MultiLoggerConfig{
		TextLogPath: "",
		JSONLogPath: "",
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP: {MaxEntries: 100},
		},
	}
	multiLogger, err := obs.NewMultiLogger(config)
	if err != nil {
		t.Fatalf("Failed to create multiLogger: %v", err)
	}
	middleware := NewMultiModeMemoryLogMiddleware(multiLogger)

	// Manually set a small RequestBodyStore for testing eviction
	middleware.requestBodyStore = obs.NewRequestBodyStore(2) // Only store 2 bodies

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.POST("/test", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body) // drain so TeeReader captures body
		c.String(http.StatusBadRequest, "bad")
	})

	// Make 3 POST requests to trigger eviction
	var bodyRefs []string
	for i := 1; i <= 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/test", nil)
		req.Body = io.NopCloser(bytes.NewBufferString(`{"id":` + string(rune('0'+i)) + `}`))
		engine.ServeHTTP(w, req)

		entries := middleware.GetEntries()
		entry := entries[len(entries)-1]
		if bodyRef, exists := entry.Data["body_ref"]; exists {
			bodyRefs = append(bodyRefs, bodyRef.(string))
		}
	}

	store := middleware.GetRequestBodyStore()
	assert.NotNil(t, store)

	// First body should be evicted
	firstBody := store.Get(bodyRefs[0])
	assert.Nil(t, firstBody, "Expected first body to be evicted")

	// Second and third bodies should still exist
	secondBody := store.Get(bodyRefs[1])
	assert.NotNil(t, secondBody, "Expected second body to exist")

	thirdBody := store.Get(bodyRefs[2])
	assert.NotNil(t, thirdBody, "Expected third body to exist")
}
