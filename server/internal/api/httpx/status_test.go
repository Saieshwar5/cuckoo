package httpx

import (
	"net/http"
	"testing"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

// The mapping is part of the public protocol: agent backends branch on these
// statuses, so a change here is a change to every integration.
func TestStatusFor(t *testing.T) {
	cases := map[domain.Kind]int{
		domain.KindInvalid:      http.StatusUnprocessableEntity,
		domain.KindUnauthorized: http.StatusUnauthorized,
		domain.KindForbidden:    http.StatusForbidden,
		domain.KindNotFound:     http.StatusNotFound,
		domain.KindConflict:     http.StatusConflict,
		domain.KindRateLimited:  http.StatusTooManyRequests,
		domain.KindInternal:     http.StatusInternalServerError,
	}

	for kind, want := range cases {
		t.Run(string(kind), func(t *testing.T) {
			if got := statusFor(kind); got != want {
				t.Errorf("statusFor(%q) = %d, want %d", kind, got, want)
			}
		})
	}
}

// An unrecognised Kind means someone added a failure class without deciding
// what it looks like over the wire. Failing closed keeps that from silently
// becoming a 200.
func TestStatusForUnknownKindIsServerError(t *testing.T) {
	if got := statusFor(domain.Kind("something_new")); got != http.StatusInternalServerError {
		t.Errorf("statusFor(unknown) = %d, want 500", got)
	}
}
