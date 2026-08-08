// Package middleware holds cross-cutting HTTP middleware — CORS, request
// logging and panic recovery — kept separate from any single domain so every
// route benefits from the same platform behavior.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
	"github.com/iig/stagex/backend/internal/platform/logger"
)

// CORS allows exactly one frontend origin (configured once, in config). This is
// deliberately strict: the frontend talks to the backend through a single,
// configurable base URL, so a single allowed origin is all we need.
func CORS(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder captures the response status for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs one structured line per request and injects a logger into
// the request context for handlers to reuse.
func RequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			ctx := logger.WithContext(r.Context(), base)
			next.ServeHTTP(rec, r.WithContext(ctx))
			base.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recover turns a handler panic into a clean 500 instead of dropping the
// connection, logging the failure for diagnosis.
func Recover(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					base.Error("panic_recovered", "value", v, "path", r.URL.Path)
					httpx.Error(w, httpx.ErrInternal("something went wrong"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies middlewares in order (first listed is outermost).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
