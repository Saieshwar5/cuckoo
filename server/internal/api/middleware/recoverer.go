package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Recoverer turns a panic into a logged 500 instead of a dead connection.
//
// A panic is always a bug, so the stack is logged in full — but the caller is
// told nothing beyond the request id, because a stack trace in a response body
// is a gift to anyone probing the server.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// A client that hangs up mid-request surfaces as this panic
				// value. Nothing is wrong with the server, and there is no
				// connection left to answer on.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				log.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", chimw.GetReqID(r.Context()),
					"stack", string(debug.Stack()),
				)

				httpx.Error(w, r, domain.Internal(nil))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
