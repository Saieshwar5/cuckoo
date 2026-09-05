// Package httpx is the only place the server writes an HTTP response.
//
// Every handler replies through JSON or Error, so the envelope, the status code
// mapping and the logging of faults are decided once. A handler that wants to
// return a different shape has to change this package, which is exactly the
// conversation worth having.
package httpx

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// JSON writes a success response.
func JSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if body == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already sent, so the response cannot be corrected.
		// Log it: a serialisation failure here is always a bug in a response type.
		slog.ErrorContext(r.Context(), "failed to encode response body",
			"error", err,
			"path", r.URL.Path,
			"request_id", middleware.GetReqID(r.Context()),
		)
	}
}

// NoContent writes a successful empty response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// errorBody is the wire shape of every failure. Stable, and part of the public
// protocol: agent backends and the app both match on `code`.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Error writes a failure response and logs anything that is our fault.
//
// An error that is not a *domain.Error is treated as an internal fault: the
// caller is told nothing specific, and the real cause goes to the log. That way
// an unclassified error can never leak a database message to a user.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	requestID := middleware.GetReqID(r.Context())
	domainErr, ok := domain.AsError(err)
	if !ok {
		domainErr = domain.Internal(err)
	}

	status := statusFor(domainErr.Kind)

	// Whole seconds, never zero: a client told to wait 0 retries at once,
	// which is exactly what the limit exists to prevent.
	if domainErr.RetryAfter > 0 {
		w.Header().Set("Retry-After",
			strconv.Itoa(int(math.Ceil(domainErr.RetryAfter.Seconds()))))
	}

	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed",
			"error", err,
			"code", domainErr.Code,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", requestID,
		)
	} else {
		slog.DebugContext(r.Context(), "request rejected",
			"code", domainErr.Code,
			"status", status,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", requestID,
		)
	}

	JSON(w, r, status, errorBody{Error: errorDetail{
		Code:      domainErr.Code,
		Message:   domainErr.Message,
		Field:     domainErr.Field,
		RequestID: requestID,
	}})
}
