package users

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Service holds the rules for user accounts.
type Service struct {
	store Store
}

// New builds the service.
func New(s Store) *Service { return &Service{store: s} }

// Create opens an account.
//
// It does not authenticate anyone: identities and sessions are the job of the
// authentication system. This exists so that an account can be created once a
// login is proven, and so tests can produce a real user through real validation.
func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	displayName, err := validateDisplayName(in.DisplayName)
	if err != nil {
		return User{}, err
	}

	locale := in.Locale
	if locale == "" {
		locale = DefaultLocale
	}
	locale, err = validateLocale(locale)
	if err != nil {
		return User{}, err
	}

	row, err := s.store.CreateUser(ctx, gen.CreateUserParams{
		ID:          domain.NewID(),
		DisplayName: displayName,
		Locale:      locale,
	})
	if err != nil {
		return User{}, domain.Internal(fmt.Errorf("create user: %w", err))
	}
	return fromRow(row), nil
}

// Get returns an account, or a not-found error if it is absent or deleted.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := s.store.GetUser(ctx, id)
	if err != nil {
		if store.IsNoRows(err) {
			return User{}, errUserNotFound()
		}
		return User{}, domain.Internal(fmt.Errorf("get user %s: %w", id, err))
	}
	return fromRow(row), nil
}

// UpdateProfile changes the fields the user supplied and leaves the rest alone.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, in UpdateProfileInput) (User, error) {
	if in.AvatarMediaID != nil {
		row, err := s.store.GetMedia(ctx, *in.AvatarMediaID)
		if err != nil && !store.IsNoRows(err) {
			return User{}, domain.Internal(fmt.Errorf("get picture %s: %w", *in.AvatarMediaID, err))
		}
		if err := media.Picture(row, err == nil, media.Owner{Kind: media.OwnerUser, ID: id}); err != nil {
			return User{}, err
		}
	}

	params := gen.UpdateUserProfileParams{ID: id, AvatarMediaID: in.AvatarMediaID}

	if in.DisplayName != nil {
		name, err := validateDisplayName(*in.DisplayName)
		if err != nil {
			return User{}, err
		}
		params.DisplayName = &name
	}

	if in.Locale != nil {
		locale, err := validateLocale(*in.Locale)
		if err != nil {
			return User{}, err
		}
		params.Locale = &locale
	}

	if params.DisplayName == nil && params.Locale == nil && params.AvatarMediaID == nil {
		return User{}, domain.Invalid("no_changes", "Provide at least one field to update.")
	}

	row, err := s.store.UpdateUserProfile(ctx, params)
	if err != nil {
		if store.IsNoRows(err) {
			return User{}, errUserNotFound()
		}
		return User{}, domain.Internal(fmt.Errorf("update user %s: %w", id, err))
	}
	return fromRow(row), nil
}

func errUserNotFound() error {
	return domain.NotFound("user_not_found", "That account does not exist.")
}

// Delete closes an account: every session is revoked, every way of signing
// in is cut so the address can start again, every contact leaves the list,
// and the row is kept under "Deleted account" so nobody else's history loses
// a name. The person's agents are retired by the agents service before this
// is called; messages stay, because the agents they were sent to have them.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.RevokeUserSessions(ctx, id); err != nil {
			return domain.Internal(fmt.Errorf("revoke sessions of %s: %w", id, err))
		}
		if err := tx.DeleteIdentities(ctx, id); err != nil {
			return domain.Internal(fmt.Errorf("delete identities of %s: %w", id, err))
		}
		if err := tx.RemoveAllContacts(ctx, id); err != nil {
			return domain.Internal(fmt.Errorf("remove contacts of %s: %w", id, err))
		}
		n, err := tx.SoftDeleteUser(ctx, id)
		if err != nil {
			return domain.Internal(fmt.Errorf("delete user %s: %w", id, err))
		}
		if n == 0 {
			return domain.NotFound("user_not_found", "That account does not exist.")
		}
		return nil
	})
}
