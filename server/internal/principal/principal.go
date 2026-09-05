// Package principal carries the identity of an authenticated caller.
//
// It is deliberately tiny and depends on nothing: the authentication mechanism
// (a development header today, signed tokens next, agent binding secrets after
// that) changes often, while the answer it produces — who is calling — does not.
// Handlers read only this, so they never change when authentication does.
package principal

import (
	"context"

	"github.com/google/uuid"
)

// Kind distinguishes the three sorts of caller the platform will serve: a
// person using the app, an agent backend, and (later) an owner managing agents
// through the management API.
type Kind string

const (
	KindUser  Kind = "user"
	KindAgent Kind = "agent"
)

// Principal is the authenticated caller. Exactly one identifier is set,
// according to Kind. SessionID is set for a person who authenticated with a
// session token, and is what signing out revokes.
type Principal struct {
	Kind      Kind
	UserID    uuid.UUID
	AgentID   uuid.UUID
	SessionID uuid.UUID
}

// User builds a principal for a person using the app.
func User(id uuid.UUID) Principal {
	return Principal{Kind: KindUser, UserID: id}
}

// UserSession builds a principal for a person holding a session.
func UserSession(userID, sessionID uuid.UUID) Principal {
	return Principal{Kind: KindUser, UserID: userID, SessionID: sessionID}
}

// Agent builds a principal for an agent backend.
func Agent(id uuid.UUID) Principal {
	return Principal{Kind: KindAgent, AgentID: id}
}

// IsUser reports whether the caller is a person.
func (p Principal) IsUser() bool { return p.Kind == KindUser }

// IsAgent reports whether the caller is an agent backend.
func (p Principal) IsAgent() bool { return p.Kind == KindAgent }

type contextKey struct{}

// NewContext returns a context carrying the principal.
func NewContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// FromContext returns the principal placed by the authentication middleware.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

// UserID returns the calling user's identifier.
//
// It reports false for an unauthenticated request or an agent caller, so a
// handler behind user authentication can treat false as a programming error
// rather than an access-control decision made in the wrong place.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	p, ok := FromContext(ctx)
	if !ok || !p.IsUser() {
		return uuid.Nil, false
	}
	return p.UserID, true
}

// SessionID returns the session the calling person holds, reporting false
// when the caller is not a person or did not authenticate with a session.
func SessionID(ctx context.Context) (uuid.UUID, bool) {
	p, ok := FromContext(ctx)
	if !ok || !p.IsUser() || p.SessionID == uuid.Nil {
		return uuid.Nil, false
	}
	return p.SessionID, true
}

// AgentID returns the calling agent's identifier, reporting false for an
// unauthenticated request or a human caller.
func AgentID(ctx context.Context) (uuid.UUID, bool) {
	p, ok := FromContext(ctx)
	if !ok || !p.IsAgent() {
		return uuid.Nil, false
	}
	return p.AgentID, true
}
