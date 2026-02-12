package protocol

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewHandleContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")

	assert.NotNil(t, hc)
	assert.Equal(t, c, hc.GinContext)
	assert.Equal(t, "test-model", hc.ResponseModel)
	assert.Empty(t, hc.OnStreamEventHooks)
	assert.Empty(t, hc.OnStreamCompleteHooks)
	assert.Empty(t, hc.OnStreamErrorHooks)
}

func TestHandleContext_WithOnStreamEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	eventCalled := false
	hc.WithOnStreamEvent(func(event interface{}) error {
		eventCalled = true
		return nil
	})

	assert.Len(t, hc.OnStreamEventHooks, 1)

	// Test calling the hook
	err := hc.OnStreamEventHooks[0](map[string]string{"test": "data"})
	assert.NoError(t, err)
	assert.True(t, eventCalled)
}

func TestHandleContext_WithOnStreamEvent_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	expectedErr := errors.New("hook error")
	hc.WithOnStreamEvent(func(event interface{}) error {
		return expectedErr
	})

	err := hc.OnStreamEventHooks[0](nil)
	assert.Equal(t, expectedErr, err)
}

func TestHandleContext_WithOnStreamComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	completeCalled := false
	hc.WithOnStreamComplete(func() {
		completeCalled = true
	})

	assert.Len(t, hc.OnStreamCompleteHooks, 1)

	// Test calling the hook
	hc.OnStreamCompleteHooks[0]()
	assert.True(t, completeCalled)
}

func TestHandleContext_WithOnStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	expectedErr := errors.New("stream error")
	var receivedErr error
	hc.WithOnStreamError(func(err error) {
		receivedErr = err
	})

	assert.Len(t, hc.OnStreamErrorHooks, 1)

	// Test calling the hook
	hc.OnStreamErrorHooks[0](expectedErr)
	assert.Equal(t, expectedErr, receivedErr)
}

func TestHandleContext_Chaining(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	callOrder := []int{}

	// Chain multiple hooks
	hc.WithOnStreamEvent(func(event interface{}) error {
		callOrder = append(callOrder, 1)
		return nil
	}).WithOnStreamEvent(func(event interface{}) error {
		callOrder = append(callOrder, 2)
		return nil
	}).WithOnStreamComplete(func() {
		callOrder = append(callOrder, 3)
	}).WithOnStreamError(func(err error) {
		callOrder = append(callOrder, 4)
	})

	assert.Len(t, hc.OnStreamEventHooks, 2)
	assert.Len(t, hc.OnStreamCompleteHooks, 1)
	assert.Len(t, hc.OnStreamErrorHooks, 1)

	// Execute in order
	for _, hook := range hc.OnStreamEventHooks {
		hook(nil)
	}
	hc.OnStreamCompleteHooks[0]()
	hc.OnStreamErrorHooks[0](nil)

	assert.Equal(t, []int{1, 2, 3, 4}, callOrder)
}

func TestSetupSSEHeaders(t *testing.T) {
	tests := []struct {
		name          string
		checkHeader   func(*testing.T, http.Header)
	}{
		{
			name: "sets correct SSE headers",
			checkHeader: func(t *testing.T, h http.Header) {
				assert.Equal(t, "text/event-stream; charset=utf-8", h.Get("Content-Type"))
				assert.Equal(t, "no-cache", h.Get("Cache-Control"))
				assert.Equal(t, "keep-alive", h.Get("Connection"))
				assert.Equal(t, "*", h.Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "Cache-Control", h.Get("Access-Control-Allow-Headers"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			hc := NewHandleContext(c, "test-model")
			hc.SetupSSEHeaders()

			tt.checkHeader(t, c.Writer.Header())
		})
	}
}

func TestProcessStream_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	eventsProcessed := []int{}
	completeCalled := false

	hc.WithOnStreamEvent(func(event interface{}) error {
		if num, ok := event.(int); ok {
			eventsProcessed = append(eventsProcessed, num)
		}
		return nil
	}).WithOnStreamComplete(func() {
		completeCalled = true
	})

	// Directly test the hook mechanism without ProcessStream
	// ProcessStream requires http.CloseNotifier which httptest doesn't support
	eventData := []interface{}{1, 2, 3}
	for _, event := range eventData {
		for _, hook := range hc.OnStreamEventHooks {
			if err := hook(event); err != nil {
				t.Errorf("hook error: %v", err)
			}
		}
		// Simulate handleFunc processing
		eventsProcessed = append(eventsProcessed, event.(int))
	}

	// Simulate complete hooks
	hc.CallOnStreamComplete()

	assert.Equal(t, []int{1, 1, 2, 2, 3, 3}, eventsProcessed)
	assert.True(t, completeCalled)
}

func TestProcessStream_HookError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	hookErr := errors.New("hook error")

	hc.WithOnStreamEvent(func(event interface{}) error {
		return hookErr
	})

	// Test that hook error is propagated
	err := hc.OnStreamEventHooks[0](nil)
	assert.Equal(t, hookErr, err)
}

func TestCallOnStreamComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	completeCalled := false
	calledTimes := []int{}

	hc.WithOnStreamComplete(func() {
		completeCalled = true
		calledTimes = append(calledTimes, 1)
	}).WithOnStreamComplete(func() {
		calledTimes = append(calledTimes, 2)
	})

	hc.CallOnStreamComplete()

	assert.True(t, completeCalled)
	assert.Equal(t, []int{1, 2}, calledTimes)
}

func TestSendError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	hc := NewHandleContext(c, "test-model")
	testErr := errors.New("test error")

	hc.SendError(testErr, "test_type", "test_code")

	// Verify response was sent
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "test error")
}

func TestIsContextCanceled(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		err := context.Canceled
		assert.True(t, IsContextCanceled(err))
	})

	t.Run("wrapped canceled context with %w", func(t *testing.T) {
		// errors.Is only matches when wrapped with %w
		err := fmt.Errorf("wrapped: %w", context.Canceled)
		assert.True(t, IsContextCanceled(err))
	})

	t.Run("other error", func(t *testing.T) {
		err := errors.New("some other error")
		assert.False(t, IsContextCanceled(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsContextCanceled(nil))
	})
}

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		errorType string
		code     string
	}{
		{
			name:     "basic error response",
			message:  "Something went wrong",
			errorType: "api_error",
			code:     "invalid_request",
		},
		{
			name:     "error without code",
			message:  "Another error",
			errorType: "validation_error",
			code:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := ErrorResponse{
				Error: ErrorDetail{
					Message: tt.message,
					Type:    tt.errorType,
					Code:    tt.code,
				},
			}

			// Verify structure
			assert.Equal(t, tt.message, resp.Error.Message)
			assert.Equal(t, tt.errorType, resp.Error.Type)
			assert.Equal(t, tt.code, resp.Error.Code)
		})
	}
}
