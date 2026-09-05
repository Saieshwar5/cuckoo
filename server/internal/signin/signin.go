// Package signin is how a person proves who they are and gets a session.
//
// It is deliberately the simplest thing that is still real: an email address
// proven by a short code, and a random session token stored hashed and
// looked up on every request, exactly as an agent's binding secret is. It
// needs no third party. Everything a more serious system adds later — another
// way to sign in, short-lived tokens with refresh, more than one device — is
// additive and leaves the app's three calls (start, verify, sign out) alone.
package signin

import (
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

const (
	// codeTTL is how long a code is good for. Long enough for a slow inbox,
	// short enough that an old email is worthless.
	codeTTL = 10 * time.Minute
	// codeAttemptsMax is how many guesses a code allows. The code space is a
	// million; five guesses makes a guess worth nothing.
	codeAttemptsMax = 5
	// sessionTTL is how long a device stays signed in without signing in
	// again. Generous, because there is no refresh yet and a phone that has
	// to sign in monthly is a phone that gets uninstalled.
	sessionTTL = 90 * 24 * time.Hour
)

// Session is one signed-in device.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// Verified is the result of a successful sign-in. Token is shown once and
// never stored in the clear. IsNew tells the app to offer profile setup.
type Verified struct {
	User    users.User
	Session Session
	Token   string
	IsNew   bool
}
