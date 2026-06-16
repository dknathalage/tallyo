package httpapi

import (
	"context"
	"log/slog"
)

// loggerCtxKey is the context key under which the request-scoped logger holder
// is stored. A dedicated unexported key type avoids collisions with other
// packages.
type loggerCtxKey struct{}

// loggerHolder wraps the request-scoped logger behind a pointer so that
// downstream middleware (RequireAuth) can enrich it AFTER RequestLogger has
// stored it, with the change visible to the original context. A plain
// context.WithValue would not propagate back up the middleware chain, so the
// final request line would miss tenant_id/user_id.
type loggerHolder struct {
	logger *slog.Logger
}

// WithLogger returns a copy of ctx carrying a holder for the given
// request-scoped logger. A nil logger is a programmer error.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		panic("httpapi.WithLogger: nil logger")
	}
	return context.WithValue(ctx, loggerCtxKey{}, &loggerHolder{logger: l})
}

// EnrichLogger replaces the request-scoped logger in ctx with the result of
// applying fn to it (typically adding attributes). It is a no-op when no holder
// is present. The mutation is visible to any code holding the same context,
// including the RequestLogger middleware that emits the final request line.
func EnrichLogger(ctx context.Context, fn func(*slog.Logger) *slog.Logger) {
	if fn == nil {
		return
	}
	h, ok := ctx.Value(loggerCtxKey{}).(*loggerHolder)
	if !ok || h == nil {
		return
	}
	enriched := fn(h.logger)
	if enriched != nil {
		h.logger = enriched
	}
}

// LoggerFrom returns the request-scoped logger stored in ctx, falling back to
// slog.Default() when none is present (e.g. background work outside a request).
// Handlers and services use this so every log line inherits request_id and,
// when authenticated, tenant_id/user_id.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if h, ok := ctx.Value(loggerCtxKey{}).(*loggerHolder); ok && h != nil && h.logger != nil {
		return h.logger
	}
	return slog.Default()
}
