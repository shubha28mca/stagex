// Package logger provides a small, dependency-free structured logger built on
// the standard library's slog. Centralizing logger construction here means the
// whole platform emits logs in one consistent JSON format that is easy to ship
// to any log aggregator.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// New builds a JSON structured logger at the requested level.
// level is one of: debug, info, warn, error (case-insensitive).
func New(level string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(h)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ctxKey is an unexported type to avoid context key collisions.
type ctxKey struct{}

// WithContext stores the logger in the context so downstream handlers and
// services can retrieve a request-scoped logger.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger stored in the context, or a default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
