package protocolserver

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/recording/capture"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// recordingMiddleware owns the lifecycle of the boundary recording
// (.design/recording.md §4): it puts a disabled capture.Trace on the request
// context, and after the handler returns — on every success, error and panic
// path — emits it if a handler enabled it. Protocol code never touches the
// Trace; only the model handlers hand over the inbound body and enable it.
func (ph *ProtocolHandler) recordingMiddleware(c *gin.Context) {
	t := capture.New()
	c.Request = c.Request.WithContext(capture.WithTrace(c.Request.Context(), t))
	defer func() {
		rid := c.GetString(constant.CtxKeyRequestID)
		if rid == "" {
			rid = uuid.NewString()
		}
		snap := t.Finish(rid)
		if snap == nil || ph.deps.TraceWriter == nil {
			return
		}
		ph.deps.TraceWriter().Emit(snap)
	}()
	c.Next()
}

// captureInbound hands the client request body, exactly as received, to the
// request's Trace. It only keeps a reference: free while recording is off.
func captureInbound(c *gin.Context, body []byte) {
	capture.FromContext(c.Request.Context()).SetInboundRequest(c.Request.Method, c.Request.URL.String(), c.Request.Header, body)
}

// enableCapture turns the request's Trace on with the effective recording
// selection (rule flag over scenario default). A disabled selection is a
// no-op; failover re-entries keep the first call's settings.
func enableCapture(c *gin.Context, mode typ.RecordingMode, scenario typ.RuleScenario, rule *typ.Rule) {
	t := capture.FromContext(c.Request.Context())
	if t == nil || !mode.Enabled() {
		return
	}
	opts := capture.EnableOptions{
		Mode:     mode,
		Scenario: string(scenario),
		Session:  typ.GetSessionID(c.Request.Context()),
	}
	if rule != nil {
		opts.Rule = rule.UUID
	}
	t.Enable(opts)
}
