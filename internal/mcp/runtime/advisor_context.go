package runtime

import "context"

type advisorContextKey struct{}

// AdvisorContext holds conversation state for the advisor tool.
type AdvisorContext struct {
	Messages      []map[string]any
	UsesRemaining int
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
