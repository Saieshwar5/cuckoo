package httpx

import (
	"net/http"

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
