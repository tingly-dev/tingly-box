package virtualserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// midStreamInjection returns the mid-stream injection configured on vm, or
// nil if no mid-stream injection applies. Pre-content injections are not
// returned by this helper — the streaming handler should consult
// vmodel.ExtractErrorInjection directly for those.
func midStreamInjection(vm any) *vmodel.ErrorInjection {
	e := vmodel.ExtractErrorInjection(vm)
	if e == nil || e.Stage != vmodel.ErrorStageMidStream {
		return nil
	}
	return e
}

// writePreContentErrorOpenAI emits an OpenAI-shaped error envelope at the
// configured HTTP status. Used for pre-content (开头) errors on the OpenAI
// Chat endpoint: handler returns the failure before any streaming starts.
func writePreContentErrorOpenAI(c *gin.Context, e *vmodel.ErrorInjection) {
	status, msg, typ := resolveErrorFields(e)
	c.JSON(status, gin.H{"error": gin.H{
		"message": msg,
		"type":    typ,
	}})
}

// writePreContentErrorAnthropic emits an Anthropic-shaped error envelope at
// the configured HTTP status (Anthropic uses {"type":"error","error":{...}}).
func writePreContentErrorAnthropic(c *gin.Context, e *vmodel.ErrorInjection) {
	status, msg, typ := resolveErrorFields(e)
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    typ,
			"message": msg,
		},
	})
}

// applyMidStreamBreakOpenAI applies a mid-stream (中间) break to an in-flight
// OpenAI SSE stream. The handler has already written one or more chunk events
// to the wire; this either emits a final SSE error frame or hijacks the TCP
// connection to simulate an abrupt upstream disconnect.
func applyMidStreamBreakOpenAI(c *gin.Context, w io.Writer, e *vmodel.ErrorInjection) {
	switch e.MidStreamMode {
	case vmodel.MidStreamModeErrorEvent:
		_, msg, typ := resolveErrorFields(e)
		payload, _ := json.Marshal(gin.H{"error": gin.H{
			"message": msg,
			"type":    typ,
		}})
		fmt.Fprintf(w, "data: %s\n\n", payload)
		c.Writer.Flush()
	case vmodel.MidStreamModeConnectionClose:
		hijackAndClose(c.Writer)
	}
}

// applyMidStreamBreakAnthropic applies a mid-stream break to an in-flight
// Anthropic SSE stream. The Anthropic streaming protocol has a dedicated
// "event: error" frame shape that we mirror here.
func applyMidStreamBreakAnthropic(c *gin.Context, w io.Writer, e *vmodel.ErrorInjection) {
	switch e.MidStreamMode {
	case vmodel.MidStreamModeErrorEvent:
		_, msg, typ := resolveErrorFields(e)
		payload, _ := json.Marshal(gin.H{
			"type": "error",
			"error": gin.H{
				"type":    typ,
				"message": msg,
			},
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		c.Writer.Flush()
	case vmodel.MidStreamModeConnectionClose:
		hijackAndClose(c.Writer)
	}
}

// resolveErrorFields fills in defaults for ErrorInjection's protocol-neutral
// fields: status defaults to 500, type to "api_error", message to a generic
// label that says where the failure was injected.
func resolveErrorFields(e *vmodel.ErrorInjection) (status int, message, typ string) {
	status = e.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	typ = e.Type
	if typ == "" {
		typ = "api_error"
	}
	message = e.Message
	if message == "" {
		if e.Stage == vmodel.ErrorStagePreContent {
			message = "simulated pre-content error"
		} else {
			message = "simulated mid-stream error"
		}
	}
	return
}

// hijackAndClose takes control of the underlying TCP connection and closes
// it. Used to simulate an upstream that abruptly drops the connection mid
// stream — exactly the failure shape the priority-routing firstChunkGate
// MUST NOT retry past.
func hijackAndClose(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil || conn == nil {
		return
	}
	_ = conn.Close()
}
