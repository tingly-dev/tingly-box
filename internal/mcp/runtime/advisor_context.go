package runtime

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

type advisorContextKey struct{}
type advisorRecordSinkKey struct{}

// AdvisorContext holds conversation state for the advisor tool.
type AdvisorContext struct {
	Messages      []map[string]any
	UsesRemaining *int
}

// WithAdvisorContext attaches advisor context to the context.
func WithAdvisorContext(ctx context.Context, ac *AdvisorContext) context.Context {
	return context.WithValue(ctx, advisorContextKey{}, ac)
}

// GetAdvisorContext retrieves advisor context from the context.
func GetAdvisorContext(ctx context.Context) (*AdvisorContext, bool) {
	ac, ok := ctx.Value(advisorContextKey{}).(*AdvisorContext)
	return ac, ok
}

// WithAdvisorRecordSink attaches a record sink to the context for advisor call recording.
func WithAdvisorRecordSink(ctx context.Context, sink *obs.Sink) context.Context {
	return context.WithValue(ctx, advisorRecordSinkKey{}, sink)
}

// GetAdvisorRecordSink retrieves the record sink from the context.
func GetAdvisorRecordSink(ctx context.Context) (*obs.Sink, bool) {
	sink, ok := ctx.Value(advisorRecordSinkKey{}).(*obs.Sink)
	return sink, ok && sink != nil
}
