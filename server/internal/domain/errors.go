// Package domain holds types shared by every business package: the error model
// and identifier handling.
//
// Errors carry a Kind (what class of failure) and a Code (a stable, machine
// readable string). Business packages return these; exactly one place — the
// HTTP layer — decides what status code a Kind maps to. No handler ever writes
// an HTTP status by hand, so the API stays consistent as it grows.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// Kind classifies a failure. It determines the HTTP status at the edge.
type Kind string

const (
	KindInternal     Kind = "internal"
	KindInvalid      Kind = "invalid"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
	KindRateLimited  Kind = "rate_limited"
)

// Error is a failure that the API can describe to a caller.
//
// Message is always safe to return over the wire. Anything sensitive belongs in
// the wrapped error, which is logged but never serialised.
type Error struct {
	Kind    Kind
	Code    string // stable identifier, e.g. "user_not_found"
	Message string // human readable, safe to expose
	Field   string // optional: the offending input field
	// RetryAfter, when set on a rate-limited error, tells the caller how
	// long to wait. It becomes the Retry-After header at the edge.
	RetryAfter time.Duration
	wrapped    error
}

func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// Wrap attaches an underlying cause for logging without changing what the
// caller is told.
func (e *Error) Wrap(err error) *Error {
	e.wrapped = err
	return e
}

// Internal reports a fault that is our problem, not the caller's. The cause is
// preserved for logs; callers receive a generic message.
func Internal(err error) *Error {
	return &Error{
		Kind:    KindInternal,
		Code:    "internal_error",
		Message: "Something went wrong on our side.",
		wrapped: err,
	}
}

// Invalid reports malformed or unacceptable input.
func Invalid(code, message string) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: message}
}

// InvalidField reports unacceptable input and names the field responsible.
func InvalidField(field, code, message string) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: message, Field: field}
}

// Unauthorized reports missing or unusable credentials.
func Unauthorized(code, message string) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Message: message}
}

// Forbidden reports valid credentials without permission for this action.
func Forbidden(code, message string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: message}
}

// NotFound reports an absent resource.
func NotFound(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// Conflict reports a request that clashes with current state.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// RateLimited reports that the caller is sending too much, and when it may
// try again.
func RateLimited(code, message string, retryAfter time.Duration) *Error {
	return &Error{Kind: KindRateLimited, Code: code, Message: message, RetryAfter: retryAfter}
}

// AsError extracts a *Error from anywhere in an error chain.
func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// KindOf returns the Kind of an error, treating anything unrecognised as
// internal. An unclassified error is a bug, and reporting it as a 500 is the
// safe default.
func KindOf(err error) Kind {
	if e, ok := AsError(err); ok {
		return e.Kind
	}
	return KindInternal
}

// CodeOf returns the stable code of an error, or "internal_error".
func CodeOf(err error) string {
	if e, ok := AsError(err); ok {
		return e.Code
	}
	return "internal_error"
}
