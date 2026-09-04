// Package middleware holds the HTTP middleware the server adds to chi's.
//
// Everything here is structured-log aware: one JSON line per request, with the
// request id that also appears in every error response, so a user reporting a
// failure hands over the exact key needed to find it.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Logger writes one structured line per request.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health checks run constantly and say nothing when they pass.
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			started := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(started).Milliseconds(),
				"request_id", chimw.GetReqID(r.Context()),
			}

			// The failure itself is logged with its cause by httpx.Error; this
			// line records the shape of the request that produced it.
			if ww.Status() >= http.StatusInternalServerError {
				log.ErrorContext(r.Context(), "request", attrs...)
				return
			}
			log.InfoContext(r.Context(), "request", attrs...)
		})
	}
}
