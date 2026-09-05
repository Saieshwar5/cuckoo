package testutil

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Response is an HTTP response captured for assertions.
type Response struct {
	t      *testing.T
	Status int
	Header http.Header
	Body   []byte
}

// errorEnvelope mirrors the shape httpx writes for every failure.
type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Field     string `json:"field"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// ExpectStatus fails the test unless the status matches, reporting the body so
// an unexpected failure explains itself without a second run.
func (r *Response) ExpectStatus(want int) *Response {
	r.t.Helper()
	if r.Status != want {
		r.t.Fatalf("expected status %d, got %d\nbody: %s", want, r.Status, r.Body)
	}
	return r
}

// Decode unmarshals a successful response body.
func (r *Response) Decode(dst any) *Response {
	r.t.Helper()
	if err := json.Unmarshal(r.Body, dst); err != nil {
		r.t.Fatalf("decode response: %v\nbody: %s", err, r.Body)
	}
	return r
}

// ExpectError asserts the status and the stable error code together, because
// either alone can match for the wrong reason.
func (r *Response) ExpectError(status int, code string) *Response {
	r.t.Helper()
	r.ExpectStatus(status)

	var env errorEnvelope
	if err := json.Unmarshal(r.Body, &env); err != nil {
		r.t.Fatalf("decode error response: %v\nbody: %s", err, r.Body)
	}
	if env.Error.Code != code {
		r.t.Fatalf("expected error code %q, got %q\nbody: %s", code, env.Error.Code, r.Body)
	}
	if env.Error.Message == "" {
		r.t.Errorf("error %q has no message; every failure must explain itself", code)
	}
	if env.Error.RequestID == "" {
		r.t.Errorf("error %q has no request_id; failures must be traceable to a log line", code)
	}
	return r
}

// ErrorField returns the field named by an error response, if any.
func (r *Response) ErrorField() string {
	r.t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(r.Body, &env); err != nil {
		r.t.Fatalf("decode error response: %v\nbody: %s", err, r.Body)
	}
	return env.Error.Field
}
