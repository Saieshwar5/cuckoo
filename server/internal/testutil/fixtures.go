package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/cuckoo-chat/cuckoo/server/internal/store"
	"github.com/cuckoo-chat/cuckoo/server/internal/users"
)

// fixtureSeq keeps generated values distinct within a test binary, so a test
// that accidentally depends on a specific name fails loudly instead of passing
// because two fixtures happened to collide.
var fixtureSeq atomic.Int64

// UserOption customises a user fixture.
type UserOption func(*users.CreateInput)

// WithDisplayName sets the fixture's name.
func WithDisplayName(name string) UserOption {
	return func(in *users.CreateInput) { in.DisplayName = name }
}

// WithLocale sets the fixture's locale.
func WithLocale(locale string) UserOption {
	return func(in *users.CreateInput) { in.Locale = locale }
}

// CreateUser inserts a user and returns it.
//
// It goes through the real service rather than writing the row directly, so a
// fixture can never hold a value that the application itself would reject —
// which is how tests end up passing against data that cannot exist.
func CreateUser(t *testing.T, db *store.Store, opts ...UserOption) users.User {
	t.Helper()

	in := users.CreateInput{
		DisplayName: fmt.Sprintf("Test User %d", fixtureSeq.Add(1)),
		Locale:      users.DefaultLocale,
	}
	for _, opt := range opts {
		opt(&in)
	}

	user, err := users.New(db).Create(context.Background(), in)
	if err != nil {
		t.Fatalf("testutil: create user fixture: %v", err)
	}
	return user
}
