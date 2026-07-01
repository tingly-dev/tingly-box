package server

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// TransformStage selects which side of the transform pipeline the recorder
// captures from.
type TransformStage int

const (
	// StagePre captures ctx.OriginalRequest before any transformation is
	// applied.
	StagePre TransformStage = iota
	// StagePost captures ctx.Request after base transformation.
	StagePost
)

// TransformRecorder is a transform.Transform that snapshots the request body
// at a given stage and stores it on a ProtocolRecorder.
type TransformRecorder struct {
	recorder *ProtocolRecorder
	c        *gin.Context
	stage    TransformStage
}

// NewTransformRecorder builds a recorder transform for the given stage.
func NewTransformRecorder(c *gin.Context, recorder *ProtocolRecorder, stage TransformStage) *TransformRecorder {
	return &TransformRecorder{
		recorder: recorder,
		c:        c,
		stage:    stage,
	}
}

func (t *TransformRecorder) Name() string {
	if t.stage == StagePre {
		return "record_pre_transform"
	}
	return "record_post_transform"
}

func (t *TransformRecorder) Apply(ctx *transform.TransformContext) error {
	if t == nil || t.recorder == nil {
		return nil
	}

	var src interface{}
	if t.stage == StagePre {
		src = ctx.OriginalRequest
	} else {
		src = ctx.Request
	}

	rec, err := t.toRecordRequest(src)
	if err != nil {
		return fmt.Errorf("failed to record %s request: %w", t.Name(), err)
	}

	if t.stage == StagePre {
		t.recorder.SetOriginalRequest(rec)
	} else {
		t.recorder.SetTransformedRequest(rec)
	}
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
