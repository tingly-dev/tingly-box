package stream

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ctxKeyLoggedError holds the last error LogRequestError recorded for the
// request, so the shared error exits (SendStreamingError, SendForwardingError,
// ...) don't log again what the call site already logged.
const ctxKeyLoggedError = "tingly_logged_request_error"

// LogRequestError records a request failure on that request's timeline in
// the Logs page. The entry goes through the request context — which routes it
// to the model_request source under the request_id — and carries the full
// error (upstream body included; the client-facing message is the redacted
// one, see .design/logging.md §0). Logging without the context sends it to
// System logs, detached from the request, which is how failures used to go
// missing from the Requests view.
//
// The same error (or one wrapping it, either way) is logged once per request:
// a converter logs the stream error and then hands the same err to
// SendStreamingError, and both must not produce a line.
func LogRequestError(c *gin.Context, err error, msg string) {
	if c == nil || c.Request == nil || err == nil {
		return
	}
	if prev, ok := c.Get(ctxKeyLoggedError); ok {
		if logged, ok := prev.(error); ok && (errors.Is(err, logged) || errors.Is(logged, err)) {
			return
		}
	}
	c.Set(ctxKeyLoggedError, err)

	fields := logrus.Fields{}
	if v, ok := c.Get(constant.CtxKeyProvider); ok {
		if p, ok := v.(*typ.Provider); ok && p != nil {
			fields["provider"] = p.Name
		}
	}
	if m := c.GetString(constant.CtxKeyModel); m != "" {
		fields["model"] = m
	}
	logrus.WithContext(c.Request.Context()).WithFields(fields).WithError(err).Error(msg)
}
