package obs

import (
	"context"
	"io"

	"github.com/sirupsen/logrus"
)

// loggerCtxKey is the context key under which the request-scoped log entry
// is stored. Using a private struct type avoids collisions with other
// packages' context keys.
type loggerCtxKey struct{}

// requestIDCtxKey is the context key under which the correlation id is stored.
type requestIDCtxKey struct{}

// discardEntry is a no-op entry returned by LogFromContext when no
// request-scoped logger is present. It is safe to chain WithField/Log on it;
// output is discarded.
var discardEntry = func() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return logrus.NewEntry(l)
}()

// ContextWithLogger returns a child context carrying the request-scoped log
// entry. Downstream packages (protocol, client) retrieve it via
// LogFromContext to emit correlated, model_request-sourced logs without
// holding a reference to the MultiLogger.
func ContextWithLogger(ctx context.Context, entry *logrus.Entry) context.Context {
	if entry == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerCtxKey{}, entry)
}

// LogFromContext returns the request-scoped log entry stored in ctx, or a
// no-op entry when none is present. The result is always safe to use.
func LogFromContext(ctx context.Context) *logrus.Entry {
	if ctx != nil {
		if e, ok := ctx.Value(loggerCtxKey{}).(*logrus.Entry); ok && e != nil {
			return e
		}
	}
	return discardEntry
}

// ContextWithRequestID returns a child context carrying the correlation id.
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
