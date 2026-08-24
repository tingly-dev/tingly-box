package protocolserver

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/recording"
)

// TransformRecorder is a transform.Transform that snapshots ctx.OriginalRequest
// (the client_request capture point — the inbound request before any
// transformation) onto a ProtocolRecorder. It runs as the first step in the
// preBase slot, before any protocol conversion.
//
// This is deliberately the only capture point left at the chain level: the
// upstream_request point moved to client.wireRecorderTransport, the
// innermost client transport layer, because the chain only ever saw a
// pre-wire SDK struct with no real headers and the wrong URL (see
// .design/recording.md).
type TransformRecorder struct {
	recorder *recording.ProtocolRecorder
	c        *gin.Context
}

// NewTransformRecorder builds the client_request capture transform.
func NewTransformRecorder(c *gin.Context, recorder *recording.ProtocolRecorder) *TransformRecorder {
	return &TransformRecorder{
		recorder: recorder,
		c:        c,
	}
}

func (t *TransformRecorder) Name() string { return "record_client_request" }

func (t *TransformRecorder) Apply(ctx *transform.TransformContext) error {
	if t == nil || t.recorder == nil {
		return nil
	}

	rec, err := t.toRecordRequest(ctx.OriginalRequest)
	if err != nil {
		return fmt.Errorf("failed to record %s request: %w", t.Name(), err)
	}
	t.recorder.SetOriginalRequest(rec)
	return nil
}

// toRecordRequest JSON-roundtrips an arbitrary request object into a
// RecordRequest, using the gin context for HTTP method/URL when available.
func (t *TransformRecorder) toRecordRequest(req interface{}) (*obs.RecordRequest, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(data, &bodyMap); err != nil {
		return nil, err
	}

	method := "POST"
	url := "/unknown"
	if t.c != nil {
		method = t.c.Request.Method
		url = t.c.Request.URL.String()
	}

	return &obs.RecordRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string),
		Body:    bodyMap,
	}, nil
}
