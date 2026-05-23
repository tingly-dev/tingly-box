package obs

import (
	"context"
)

// requestIDCtxKey is the context key under which the correlation id is stored.
// Using a private struct type avoids collisions with other packages' keys.
type requestIDCtxKey struct{}

// ContextWithRequestID returns a child context carrying the correlation id.
// Request-scoped code logs via logrus.WithContext(ctx); the MultiLogger hook
// reads the id back with RequestIDFromContext to route the entry to the
// model_request source and stamp it with the id.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDCtxKey{}, id)
}

// RequestIDFromContext returns the correlation id stored in ctx, or "".
func RequestIDFromContext(ctx context.Context) string {
	if ctx != nil {
		if id, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
			return id
		}
	}
	return ""
}
