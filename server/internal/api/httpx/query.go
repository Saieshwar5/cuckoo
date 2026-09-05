package httpx

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// QueryID reads an optional prefixed identifier from the query string.
//
// It returns nil when the parameter is absent or empty. A malformed value is
// reported against the parameter's own name, so a client learns that
// "?before=" was wrong rather than that some "id" was.
func QueryID(r *http.Request, name, prefix string) (*uuid.UUID, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}

	id, err := domain.ParseID(prefix, raw)
	if err != nil {
		if e, ok := domain.AsError(err); ok {
			return nil, domain.InvalidField(name, e.Code, e.Message)
		}
		return nil, err
	}
	return &id, nil
}

// QueryInt reads an optional integer from the query string, returning zero
// when it is absent. A value that is not a whole number is reported against
// the parameter's name with the code "invalid_<name>".
func QueryInt(r *http.Request, name string) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.InvalidField(name, "invalid_"+name,
			fmt.Sprintf("%s must be a whole number.", capitalize(name)))
	}
	return n, nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
