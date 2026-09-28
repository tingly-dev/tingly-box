package stream

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newLogTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/tingly/openai/v1/chat/completions", nil)
	c.Request = req.WithContext(obs.ContextWithRequestID(req.Context(), "req-1"))
	return c
}

func TestLogRequestError_LandsOnRequestTimeline(t *testing.T) {
	hook := test.NewGlobal()
	defer hook.Reset()
	c := newLogTestContext()
	c.Set(constant.CtxKeyProvider, &typ.Provider{Name: "openai"})
	c.Set(constant.CtxKeyModel, "gpt-image-2")

	LogRequestError(c, errors.New("moderation_blocked"), "failed to forward request")

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Equal(t, "req-1", obs.RequestIDFromContext(entry.Context))
	assert.Equal(t, "openai", entry.Data["provider"])
	assert.Equal(t, "gpt-image-2", entry.Data["model"])
	assert.NotNil(t, entry.Data[logrus.ErrorKey])
}

// A converter logs the stream error and then hands the same err (or a wrap
// of it) to SendStreamingError: one line, not two.
func TestLogRequestError_SameErrorLoggedOnce(t *testing.T) {
	hook := test.NewGlobal()
	defer hook.Reset()
	c := newLogTestContext()
	base := errors.New("upstream 500")

	LogRequestError(c, base, "OpenAI stream error")
	SendStreamingError(c, base)
	LogRequestError(c, fmt.Errorf("stream error: %w", base), "wrapped")
	assert.Len(t, hook.AllEntries(), 1)

	LogRequestError(c, errors.New("different"), "other")
	assert.Len(t, hook.AllEntries(), 2)
}
