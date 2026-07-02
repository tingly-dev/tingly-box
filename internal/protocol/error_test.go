package protocol

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAnthropicUpstreamError(t *testing.T, status int, body string) *anthropic.Error {
	t.Helper()
	e := &anthropic.Error{StatusCode: status}
	require.NoError(t, e.UnmarshalJSON([]byte(body)))
	return e
}

func TestBuildAnthropicError_SameProtocolUpstream(t *testing.T) {
	upstream := newAnthropicUpstreamError(t, 429,
		`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)

	body := BuildAnthropicError(upstream, 429)

	assert.Equal(t, "error", body.Type)
	assert.Equal(t, AnthropicErrRateLimit, body.Error.Type)
	assert.Equal(t, "slow down", body.Error.Message)
}

func TestBuildAnthropicError_CrossProtocolUpstream(t *testing.T) {
	// openai.Error's Type is a free string, not validated against Anthropic's
	// enum; when it doesn't validate, the status-derived type wins.
	upstream := &openai.Error{StatusCode: 429, Type: "requests", Message: "rate limited by openai"}

	body := BuildAnthropicError(upstream, 429)

	assert.Equal(t, "error", body.Type)
	assert.Equal(t, AnthropicErrRateLimit, body.Error.Type) // status-derived fallback
	assert.Equal(t, "rate limited by openai", body.Error.Message)
}

func TestBuildAnthropicError_InternalError(t *testing.T) {
	err := errors.New("json: cannot marshal")

	body := BuildAnthropicError(err, http.StatusInternalServerError)

	assert.Equal(t, "error", body.Type)
	assert.Equal(t, AnthropicErrAPI, body.Error.Type)
	assert.Equal(t, "json: cannot marshal", body.Error.Message)
}

func TestBuildOpenAIError_SameProtocolUpstream(t *testing.T) {
	upstream := &openai.Error{StatusCode: 401, Type: "invalid_api_key", Message: "bad key", Code: "invalid_api_key"}

	body := BuildOpenAIError(upstream, 401)

	assert.Equal(t, "invalid_api_key", body.Error.Type)
	assert.Equal(t, "bad key", body.Error.Message)
	require.NotNil(t, body.Error.Code)
	assert.Equal(t, "invalid_api_key", *body.Error.Code)
}

func TestBuildOpenAIError_CrossProtocolUpstream(t *testing.T) {
	upstream := newAnthropicUpstreamError(t, 529,
		`{"type":"error","error":{"type":"overloaded_error","message":"upstream is overloaded"}}`)

	body := BuildOpenAIError(upstream, 529)

	// OpenAI has no enum, so the upstream's real type/message pass through as-is.
	assert.Equal(t, "overloaded_error", body.Error.Type)
	assert.Equal(t, "upstream is overloaded", body.Error.Message)
}

func TestBuildOpenAIError_InternalError(t *testing.T) {
	err := errors.New("boom")

	body := BuildOpenAIError(err, http.StatusBadRequest)

	assert.Equal(t, "invalid_request_error", body.Error.Type)
	assert.Equal(t, "boom", body.Error.Message)
	assert.Nil(t, body.Error.Code)
	assert.Nil(t, body.Error.Param)
}

func TestSendAnthropicError_HasTopLevelType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	upstream := newAnthropicUpstreamError(t, 401,
		`{"type":"error","error":{"type":"authentication_error","message":"bad token"}}`)

	SendAnthropicError(c, upstream, "Failed to forward request")

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	assert.Equal(t, "error", decoded["type"])
	errObj := decoded["error"].(map[string]any)
	assert.Equal(t, "authentication_error", errObj["type"])
	assert.Contains(t, errObj["message"], "bad token")
	assert.Contains(t, errObj["message"], "Failed to forward request")
}

func TestSendOpenAIError_NoTopLevelType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	upstream := &openai.Error{StatusCode: 429, Type: "rate_limit_exceeded", Message: "too many requests"}

	SendOpenAIError(c, upstream, "")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	_, hasTopLevelType := decoded["type"]
	assert.False(t, hasTopLevelType, "OpenAI error body should not have a top-level type field")
	errObj := decoded["error"].(map[string]any)
	assert.Equal(t, "rate_limit_exceeded", errObj["type"])
	assert.Equal(t, "too many requests", errObj["message"])
}
