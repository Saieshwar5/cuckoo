// Package users owns everything about a person's account.
//
// It is the worked example every other business package copies: a Store
// interface naming only the queries it uses, a domain type that is not the
// database row, validation that returns domain errors, and no knowledge of HTTP.
package users

import (
	"context"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Store is the slice of the database this package needs.
//
// Naming the queries rather than accepting *store.Store keeps the dependency
// visible and lets tests substitute a fake without a database when that is the
// cheaper test. Any package may satisfy it; only store.Store does in production.
type Store interface {
	CreateUser(ctx context.Context, arg gen.CreateUserParams) (gen.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (gen.User, error)
	UpdateUserProfile(ctx context.Context, arg gen.UpdateUserProfileParams) (gen.User, error)
	GetMedia(ctx context.Context, id uuid.UUID) (gen.Medium, error)
	// WithTx runs fn inside one transaction, for the changes that must land
	// together or not at all.
	WithTx(ctx context.Context, fn func(*store.Store) error) error
}

// User is a person's account as the rest of the system understands it.
//
// It is a separate type from the generated database row on purpose: column
// names are free to change, and the API's shape is decided by the API layer,
// not by whatever the table happens to look like this month.
type User struct {
	ID          uuid.UUID
	DisplayName string
	Locale      string
	// Their photo, or nil for the initials disc. Unlike an agent's logo it
	// is not public: it is behind the session that owns it.
	AvatarMediaID *uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func fromRow(row gen.User) User {
	return User{
		ID:            row.ID,
		DisplayName:   row.DisplayName,
		Locale:        row.Locale,
		AvatarMediaID: row.AvatarMediaID,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

// CreateInput is the data needed to open an account.
type CreateInput struct {
	DisplayName string
	Locale      string
}

// UpdateProfileInput describes a partial update: a nil field is left unchanged,
// which is what lets the app send only what the user actually edited.
type UpdateProfileInput struct {
	AvatarMediaID *uuid.UUID
	DisplayName   *string
	Locale        *string
}
