package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func anthropicErrorEngine(handler gin.HandlerFunc, auth ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	chain := append([]gin.HandlerFunc{anthropicErrorConventions}, auth...)
	e.POST("/v1/messages", append(chain, handler)...)
	return e
}

func doAnthropicError(e *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	return w
}

func TestAnthropicErrorConventions_EnvelopeAndShouldRetry(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		wantNoRetry bool
	}{
		{"bad request is terminal", http.StatusBadRequest, true},
		{"unauthorized is terminal", http.StatusUnauthorized, true},
		{"forbidden is terminal", http.StatusForbidden, true},
		{"not implemented is terminal", http.StatusNotImplemented, true},
		{"rate limit stays retryable", http.StatusTooManyRequests, false},
		{"timeout stays retryable", http.StatusRequestTimeout, false},
		{"upstream 5xx stays retryable", http.StatusBadGateway, false},
		{"overloaded stays retryable", 529, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
				c.JSON(tt.status, ErrorResponse{Error: ErrorDetail{Message: "boom", Type: "api_error"}})
			}))
			require.Equal(t, tt.status, w.Code)
			assert.Equal(t, "error", gjson.Get(w.Body.String(), "type").String())
			assert.Equal(t, "boom", gjson.Get(w.Body.String(), "error.message").String())
			assert.Equal(t, "api_error", gjson.Get(w.Body.String(), "error.type").String())
			if tt.wantNoRetry {
				assert.Equal(t, "false", w.Header().Get("x-should-retry"))
			} else {
				assert.Empty(t, w.Header().Get("x-should-retry"))
			}
		})
	}
}

// Authentication fails before any handler runs; it must follow the same rules.
func TestAnthropicErrorConventions_CoversAuthMiddleware(t *testing.T) {
	auth := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrorDetail{Message: "Invalid token", Type: "invalid_request_error"}})
	}
	w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) { t.Fatal("handler must not run") }, auth))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "false", w.Header().Get("x-should-retry"))
	assert.Equal(t, "error", gjson.Get(w.Body.String(), "type").String())
}

func TestAnthropicErrorConventions_LeavesOtherResponsesAlone(t *testing.T) {
	t.Run("success body untouched", func(t *testing.T) {
		w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"error": gin.H{"message": "not an error"}})
		}))
		assert.False(t, gjson.Get(w.Body.String(), "type").Exists())
		assert.Empty(t, w.Header().Get("x-should-retry"))
	})
	t.Run("upstream envelope and header kept", func(t *testing.T) {
		w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
			c.Header("x-should-retry", "true")
			c.Data(http.StatusBadRequest, "application/json", []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long"}}`))
		}))
		assert.JSONEq(t, `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long"}}`, w.Body.String())
		assert.Equal(t, "true", w.Header().Get("x-should-retry"), "an explicit upstream decision wins")
	})
	t.Run("non-json error untouched", func(t *testing.T) {
		w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
			c.String(http.StatusBadGateway, "bad gateway")
		}))
		assert.Equal(t, "bad gateway", w.Body.String())
	})
	t.Run("streaming success", func(t *testing.T) {
		w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.String(http.StatusOK, "event: ping\ndata: {}\n\n")
			c.Writer.Flush()
		}))
		assert.Equal(t, "event: ping\ndata: {}\n\n", w.Body.String())
	})
}

func TestAnthropicErrorConventions_CarriesRetryAfter(t *testing.T) {
	w := doAnthropicError(anthropicErrorEngine(func(c *gin.Context) {
		c.Header("Retry-After", "30")
		c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: ErrorDetail{Message: "slow down", Type: "rate_limit_error"}})
	}))
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
	assert.Equal(t, "error", gjson.Get(w.Body.String(), "type").String())
}
