package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func decodeBody(t *testing.T, body, contentType string) (sample, error) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	var dst sample
	err := Decode(httptest.NewRecorder(), req, &dst)
	return dst, err
}

func TestDecodeReadsValidJSON(t *testing.T) {
	got, err := decodeBody(t, `{"name":"Priya","count":3}`, "application/json")
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if got.Name != "Priya" || got.Count != 3 {
		t.Errorf("Decode = %+v, want {Priya 3}", got)
	}
}

func TestDecodeAcceptsContentTypeWithCharset(t *testing.T) {
	if _, err := decodeBody(t, `{"name":"x"}`, "application/json; charset=utf-8"); err != nil {
		t.Errorf("Decode rejected a charset parameter: %v", err)
	}
}

// A misspelled field silently ignored looks, to whoever sent it, exactly like a
// server that accepted the request and did nothing. Rejecting it turns a
// mystery into a message.
func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := decodeBody(t, `{"name":"Priya","nickname":"P"}`, "application/json")
	if err == nil {
		t.Fatal("Decode accepted an unknown field")
	}

	d, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("error is not a domain error: %v", err)
	}
	if d.Code != "unknown_field" {
		t.Errorf("Code = %q, want unknown_field", d.Code)
	}
	if d.Field != "nickname" {
		t.Errorf("Field = %q, want nickname; the caller needs to know which field", d.Field)
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		contentType string
		wantCode    string
	}{
		{"malformed json", `{"name":`, "application/json", "invalid_body"},
		{"empty body", ``, "application/json", "invalid_body"},
		{"wrong type", `{"count":"three"}`, "application/json", "invalid_body"},
		{"not an object", `"just a string"`, "application/json", "invalid_body"},
		{"trailing object", `{"name":"a"}{"name":"b"}`, "application/json", "invalid_body"},
		{"wrong content type", `{"name":"a"}`, "text/plain", "unsupported_media_type"},
		{"form content type", `{"name":"a"}`, "application/x-www-form-urlencoded", "unsupported_media_type"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeBody(t, tc.body, tc.contentType)
			if err == nil {
				t.Fatalf("Decode accepted %s", tc.name)
			}
			if got := domain.CodeOf(err); got != tc.wantCode {
				t.Errorf("Code = %q, want %q (error: %v)", got, tc.wantCode, err)
			}
			if domain.KindOf(err) != domain.KindInvalid {
				t.Errorf("Kind = %q, want invalid", domain.KindOf(err))
			}
		})
	}
}

// A missing Content-Type is tolerated so that a quick curl without -H still
// works. The body still has to be JSON.
func TestDecodeAllowsMissingContentType(t *testing.T) {
	if _, err := decodeBody(t, `{"name":"Priya"}`, ""); err != nil {
		t.Errorf("Decode rejected a request with no Content-Type: %v", err)
	}
}

// The body limit is what stops a single request from exhausting server memory.
func TestDecodeRejectsOversizedBody(t *testing.T) {
	huge := `{"name":"` + strings.Repeat("a", maxRequestBody+1) + `"}`

	_, err := decodeBody(t, huge, "application/json")
	if err == nil {
		t.Fatal("Decode accepted a body over the limit")
	}
	if got := domain.CodeOf(err); got != "body_too_large" {
		t.Errorf("Code = %q, want body_too_large (error: %v)", got, err)
	}
}
