package httpx

import (
	"net/http"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

// statusFor maps a failure class to an HTTP status.
//
// This is the single table that decides what a client sees. Business packages
// choose a Kind; nothing else in the server names a status code.
func statusFor(kind domain.Kind) int {
	switch kind {
	case domain.KindInvalid:
		// 422 rather than 400: the request parsed correctly, its contents were
		// unacceptable. 400 is reserved for a body we could not read at all.
		return http.StatusUnprocessableEntity
	case domain.KindUnauthorized:
		return http.StatusUnauthorized
	case domain.KindForbidden:
		return http.StatusForbidden
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindRateLimited:
		return http.StatusTooManyRequests
	case domain.KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
