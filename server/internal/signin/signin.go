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

	// pushTokenMax bounds what a device may claim its address is. Expo's are
	// far shorter; this is the column's limit, not a guess at the format,
	// because the hub hands the token back to Expo and never reads inside it.
	pushTokenMax = 256
	// sessionTTL is how long a device stays signed in without signing in
	// again. Generous, because there is no refresh yet and a phone that has
	// to sign in monthly is a phone that gets uninstalled.
	sessionTTL = 90 * 24 * time.Hour
	// devicesMax is how many devices one person may be signed in on at
	// once. Past it, the one that has gone longest without being used is
	// signed out, so a phone that was replaced makes room for its
	// replacement without anybody noticing.
	devicesMax = 10
	// seenInterval is how stale a session's last-seen time may get before
	// a request refreshes it. A write per request would be a write per
	// request; an hour is close enough for a list that says "yesterday".
	seenInterval = time.Hour
)

// Session is one signed-in device.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// Device is one signed-in device as its owner sees it in Settings. LastSeen
// is nil for a session that has not carried a request since it was made.
type Device struct {
	ID        uuid.UUID
	Name      string
	LastSeen  *time.Time
	CreatedAt time.Time
	ExpiresAt time.Time
	// Current marks the device asking, which the app draws differently and
	// puts first.
	Current bool
}

// Verified is the result of a successful sign-in. Token is shown once and
// never stored in the clear. IsNew tells the app to offer profile setup.
type Verified struct {
	User    users.User
	Session Session
	Token   string
	IsNew   bool
}
