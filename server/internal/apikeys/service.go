package apikeys

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Store is the slice of the database this package uses.
type Store interface {
	CreateAPIKey(ctx context.Context, arg gen.CreateAPIKeyParams) (gen.ApiKey, error)
	ListAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]gen.ApiKey, error)
	AuthenticateAPIKey(ctx context.Context, keyHash []byte) (gen.AuthenticateAPIKeyRow, error)
	TouchAPIKey(ctx context.Context, id uuid.UUID) error
	RevokeAPIKey(ctx context.Context, arg gen.RevokeAPIKeyParams) (int64, error)
}

// Service holds the rules for API keys.
type Service struct {
	store Store
}

// New builds the service.
func New(st Store) *Service { return &Service{store: st} }

// Create issues a key for the caller's account. The key itself is returned
// exactly once; only its hash is kept, so it can never be shown again.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, name string) (Key, string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return Key{}, "", domain.InvalidField("name", "invalid_name", "Give the key a name so you know what it is for.")
	case utf8.RuneCountInString(name) > maxNameLen:
		return Key{}, "", domain.InvalidField("name", "invalid_name",
			fmt.Sprintf("The name must be %d characters or fewer.", maxNameLen))
	}

	plaintext, hash := domain.NewSecret(domain.PrefixAPIKeySecret)
	row, err := s.store.CreateAPIKey(ctx, gen.CreateAPIKeyParams{
		ID:      domain.NewID(),
		UserID:  userID,
		Name:    name,
		KeyHash: hash,
	})
	if err != nil {
		return Key{}, "", domain.Internal(fmt.Errorf("create api key for %s: %w", userID, err))
	}
	return fromRow(row), plaintext, nil
}

// ListMine returns the caller's keys, newest first, revoked ones included.
func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]Key, error) {
	rows, err := s.store.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list api keys of %s: %w", userID, err))
	}
	out := make([]Key, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Revoke ends a key. Only the account it belongs to may end it, and ending
// one that is already ended is a not-found rather than a silent success:
// the caller asked to change something that was not there.
func (s *Service) Revoke(ctx context.Context, userID, keyID uuid.UUID) error {
	n, err := s.store.RevokeAPIKey(ctx, gen.RevokeAPIKeyParams{ID: keyID, UserID: userID})
	if err != nil {
		return domain.Internal(fmt.Errorf("revoke api key %s: %w", keyID, err))
	}
	if n == 0 {
		return domain.NotFound("key_not_found", "No such live API key.")
	}
	return nil
}

// Resolve identifies the account a bearer key acts for.
//
// This is the authentication path for a company's servers, so it does the
// minimum: hash, one indexed lookup, done. Recording that the key was used
// is a second statement that writes at most once a minute, and its failure
// is logged rather than passed on: a request must not fail because we could
// not update a timestamp.
func (s *Service) Resolve(ctx context.Context, key string) (uuid.UUID, error) {
	if !strings.HasPrefix(key, domain.PrefixAPIKeySecret+"_") {
		return uuid.Nil, errBadKey()
	}
	row, err := s.store.AuthenticateAPIKey(ctx, domain.HashSecret(key))
	if err != nil {
		if store.IsNoRows(err) {
			return uuid.Nil, errBadKey()
		}
		return uuid.Nil, domain.Internal(fmt.Errorf("authenticate api key: %w", err))
	}
	if err := s.store.TouchAPIKey(ctx, row.ID); err != nil {
		slog.WarnContext(ctx, "could not record api key use", "key", row.ID, "error", err)
	}
	return row.UserID, nil
}

func errBadKey() error {
	return domain.Unauthorized("invalid_credentials",
		"That API key is not valid. It may have been revoked.")
}
