package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
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

// AgentOption customises an agent fixture.
type AgentOption func(*agents.CreateInput)

// WithHandle sets the fixture's handle.
func WithHandle(handle string) AgentOption {
	return func(in *agents.CreateInput) { in.Handle = handle }
}

// WithAgentName sets the fixture's display name.
func WithAgentName(name string) AgentOption {
	return func(in *agents.CreateInput) { in.DisplayName = name }
}

// CreateAgent inserts an agent owned by the given user, through the real
// service so it can only hold values the application accepts.
func CreateAgent(t *testing.T, db *store.Store, owner users.User, opts ...AgentOption) agents.Agent {
	t.Helper()

	n := fixtureSeq.Add(1)
	in := agents.CreateInput{
		Handle:      fmt.Sprintf("test-agent-%d", n),
		DisplayName: fmt.Sprintf("Test Agent %d", n),
		Description: "A fixture.",
	}
	for _, opt := range opts {
		opt(&in)
	}

	agent, err := agents.New(db).Create(context.Background(), owner.ID, in)
	if err != nil {
		t.Fatalf("testutil: create agent fixture: %v", err)
	}
	return agent
}

// BindAgent gives an agent a socket binding and returns it with its secret —
// the one place outside the management API where the plaintext is visible.
func BindAgent(t *testing.T, db *store.Store, agent agents.Agent) (agents.Binding, string) {
	t.Helper()

	binding, secret, err := agents.New(db).SetBinding(context.Background(), agent.OwnerID, agent.ID,
		agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("testutil: bind agent fixture: %v", err)
	}
	return binding, secret
}
