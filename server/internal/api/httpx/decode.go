package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

// maxRequestBody caps how much a caller can make the server read.
//
// One megabyte is far more than any JSON request needs — media is uploaded
// directly to object storage, never through a request body.
const maxRequestBody = 1 << 20

// Decode reads a JSON request body into dst.
//
// Unknown fields are rejected rather than ignored. A misspelled field would
// otherwise be silently discarded and look, to whoever sent it, exactly like a
// server that quietly did nothing — the worst failure an API can have.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType := strings.TrimSpace(strings.Split(ct, ";")[0]); mediaType != "application/json" {
			return domain.Invalid("unsupported_media_type",
				"Request body must be application/json.")
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}

	// A second value in the stream means the caller sent something we would
	// only half-apply. Refuse the whole request.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Invalid("invalid_body", "Request body must contain a single JSON object.")
	}

	return nil
}

func decodeError(err error) error {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
		maxErr    *http.MaxBytesError
	)

	switch {
	case errors.As(err, &syntaxErr):
		return domain.Invalid("invalid_body",
			fmt.Sprintf("Request body is not valid JSON (at byte %d).", syntaxErr.Offset))

	case errors.As(err, &typeErr):
		return domain.InvalidField(typeErr.Field, "invalid_body",
			fmt.Sprintf("Field %q must be a %s.", typeErr.Field, typeErr.Type))

	case errors.As(err, &maxErr):
		return domain.Invalid("body_too_large", "Request body is too large.")

	case errors.Is(err, io.EOF):
		return domain.Invalid("invalid_body", "Request body is empty.")

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return domain.InvalidField(field, "unknown_field",
			fmt.Sprintf("Field %q is not recognised.", field))

	default:
		return domain.Invalid("invalid_body", "Request body could not be read.")
	}
}
