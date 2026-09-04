package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestConstructorsSetKindAndCode(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		kind Kind
	}{
		{"invalid", Invalid("bad_input", "no"), KindInvalid},
		{"unauthorized", Unauthorized("no_creds", "no"), KindUnauthorized},
		{"forbidden", Forbidden("nope", "no"), KindForbidden},
		{"not found", NotFound("gone", "no"), KindNotFound},
		{"conflict", Conflict("clash", "no"), KindConflict},
		{"rate limited", RateLimited("slow_down", "no"), KindRateLimited},
		{"internal", Internal(errors.New("boom")), KindInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Kind != tc.kind {
				t.Errorf("Kind = %q, want %q", tc.err.Kind, tc.kind)
			}
			if tc.err.Code == "" {
				t.Error("Code is empty; codes are what clients match on")
			}
			if tc.err.Message == "" {
				t.Error("Message is empty; every failure must explain itself")
			}
		})
	}
}

func TestInvalidFieldNamesTheField(t *testing.T) {
	err := InvalidField("display_name", "invalid_display_name", "too long")
	if err.Field != "display_name" {
		t.Errorf("Field = %q, want %q", err.Field, "display_name")
	}
}

// An unclassified error must be treated as our fault, never as the caller's.
// Defaulting the other way would turn an unexpected bug into a 4xx and hide it.
func TestKindOfTreatsUnknownErrorsAsInternal(t *testing.T) {
	if got := KindOf(errors.New("something unexpected")); got != KindInternal {
		t.Errorf("KindOf(plain error) = %q, want %q", got, KindInternal)
	}
	if got := CodeOf(errors.New("something unexpected")); got != "internal_error" {
		t.Errorf("CodeOf(plain error) = %q, want %q", got, "internal_error")
	}
	if got := KindOf(nil); got != KindInternal {
		t.Errorf("KindOf(nil) = %q, want %q", got, KindInternal)
	}
}

// Errors travel up through layers that wrap them. The classification has to
// survive that, or the HTTP layer turns a 404 into a 500 somewhere in between.
func TestKindSurvivesWrapping(t *testing.T) {
	base := NotFound("user_not_found", "no such user")
	wrapped := fmt.Errorf("loading profile: %w", fmt.Errorf("querying user: %w", base))

	if got := KindOf(wrapped); got != KindNotFound {
		t.Errorf("KindOf(wrapped) = %q, want %q", got, KindNotFound)
	}
	if got := CodeOf(wrapped); got != "user_not_found" {
		t.Errorf("CodeOf(wrapped) = %q, want %q", got, "user_not_found")
	}

	found, ok := AsError(wrapped)
	if !ok {
		t.Fatal("AsError did not find the domain error in the chain")
	}
	if found != base {
		t.Error("AsError returned a different error than the one wrapped")
	}
}

// The cause of an internal error must stay reachable for logs, while never
// being part of what the caller is told.
func TestInternalPreservesCauseButNotInMessage(t *testing.T) {
	cause := errors.New("pq: connection refused on 10.0.0.4:5432")
	err := Internal(cause)

	if !errors.Is(err, cause) {
		t.Error("Internal lost the underlying cause; it must remain loggable")
	}
	if err.Message == cause.Error() {
		t.Error("Internal exposed the cause as the client-facing message")
	}
}

func TestWrapAttachesCause(t *testing.T) {
	cause := errors.New("timeout")
	err := NotFound("user_not_found", "no such user").Wrap(cause)

	if !errors.Is(err, cause) {
		t.Error("Wrap did not attach the cause")
	}
	if err.Kind != KindNotFound {
		t.Errorf("Wrap changed the Kind to %q", err.Kind)
	}
}
