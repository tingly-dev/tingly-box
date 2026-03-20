package server

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// PreTransformRecorder captures the original request before any transformations
// This is the first transform in the chain, recording the request as received from the client
type PreTransformRecorder struct {
	recorder interface{} // Can be *ProtocolRecorder or *ScenarioRecorder
	c        *gin.Context
}

// NewPreTransformRecorder creates a new PreTransformRecorder
func NewPreTransformRecorder(c *gin.Context, recorder interface{}) *PreTransformRecorder {
	return &PreTransformRecorder{
		recorder: recorder,
		c:        c,
	}
}

// Name returns the name of this transform
func (t *PreTransformRecorder) Name() string {
	return "record_pre_transform"
}

// Apply captures the original request before any transformations
// The original request is stored in ctx.OriginalRequest
func (t *PreTransformRecorder) Apply(ctx *transform.TransformContext) error {
	if t.recorder == nil {
		return nil
	}

	// Convert the original request to a recordable format
	reqRecord, err := t.requestToRecord(ctx.OriginalRequest)
	if err != nil {
		return fmt.Errorf("failed to record original request: %w", err)
	}

	// Store in the V2 recorder if available
	if rec, ok := t.recorder.(*ProtocolRecorder); ok {
		rec.SetOriginalRequest(reqRecord)
	}

	return nil
}

// requestToRecord converts a request object to RecordRequest format
func (t *PreTransformRecorder) requestToRecord(req interface{}) (*obs.RecordRequest, error) {
	// Convert to JSON then to map for consistent storage
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(data, &bodyMap); err != nil {
		return nil, err
	}

	// Get HTTP method and URL from gin context if available
	method := "POST"
	url := "/unknown"
	if t.c != nil {
		method = t.c.Request.Method
		url = t.c.Request.URL.String()
	}

	return &obs.RecordRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string), // Headers are recorded at HTTP level
		Body:    bodyMap,
	}, nil
}

// PostTransformRecorder captures the request after base transformation
// This records the request after protocol conversion (e.g., Anthropic → OpenAI)
type PostTransformRecorder struct {
	recorder interface{} // Can be *ProtocolRecorder or *ScenarioRecorder
	c        *gin.Context
}

// NewPostTransformRecorder creates a new PostTransformRecorder
func NewPostTransformRecorder(recorder interface{}, c *gin.Context) *PostTransformRecorder {
	return &PostTransformRecorder{
		recorder: recorder,
		c:        c,
	}
}

// Name returns the name of this transform
func (t *PostTransformRecorder) Name() string {
	return "record_post_transform"
}

// Apply captures the request after base transformation
// The transformed request is in ctx.Request (after BaseTransform)
func (t *PostTransformRecorder) Apply(ctx *transform.TransformContext) error {
	if t.recorder == nil {
		return nil
	}

	// Convert the transformed request to a recordable format
	reqRecord, err := t.requestToRecord(ctx.Request)
	if err != nil {
		return fmt.Errorf("failed to record transformed request: %w", err)
	}

	// Store in the V2 recorder if available
	if v2Rec, ok := t.recorder.(*ProtocolRecorder); ok {
		v2Rec.SetTransformedRequest(reqRecord)
	}

	return nil
}

// requestToRecord converts a request object to RecordRequest format
func (t *PostTransformRecorder) requestToRecord(req interface{}) (*obs.RecordRequest, error) {
	// Convert to JSON then to map for consistent storage
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(data, &bodyMap); err != nil {
		return nil, err
	}

	// Get HTTP method and URL from gin context if available
	method := "POST"
	url := "/unknown"
	if t.c != nil {
		method = t.c.Request.Method
		url = t.c.Request.URL.String()
	}

	return &obs.RecordRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string), // Headers are recorded at HTTP level
		Body:    bodyMap,
	}, nil
}
