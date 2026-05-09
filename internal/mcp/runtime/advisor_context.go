package runtime

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/obs"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

type AdvisorContext = coretool.AdvisorContext

var WithAdvisorContext = coretool.WithAdvisorContext
var GetAdvisorContext = coretool.GetAdvisorContext

// WithAdvisorDepth sets the adviser call depth in context.
func WithAdvisorDepth(ctx context.Context, depth int) context.Context {
	return coretool.WithAdvisorDepth(ctx, depth)
}

// GetAdvisorDepth retrieves the current adviser call depth from context.
func GetAdvisorDepth(ctx context.Context) int {
	return coretool.GetAdvisorDepth(ctx)
}

// WithAdvisorRecordSink attaches a record sink to the context for advisor call recording.
func WithAdvisorRecordSink(ctx context.Context, sink *obs.Sink) context.Context {
	return coretool.WithAdvisorRecordSink(ctx, sink)
}

// GetAdvisorRecordSink retrieves the record sink from the context.
func GetAdvisorRecordSink(ctx context.Context) (*obs.Sink, bool) {
	sink, ok := coretool.GetAdvisorRecordSink(ctx)
	if !ok {
		return nil, false
	}
	typed, ok := sink.(*obs.Sink)
	return typed, ok && typed != nil
}
