package auth

import (
	"net/http"
	"strings"

	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// Authenticator is anything that identifies a caller. It is the shape the
// HTTP middleware requires, named here so this package can compose its own
// without depending on the layer above it.
type Authenticator interface {
	Authenticate(r *http.Request) (principal.Principal, error)
}

// KeyOrUser is the management API's authenticator: an API key when one is
// presented, and whatever identifies a person otherwise.
//
// The two produce the same principal — the account — so no handler below
// knows or cares which arrived. The prefix decides: a credential that
// announces itself as a key is resolved as a key and never falls through to
// the session path, so a revoked key fails as a revoked key rather than as
// a malformed session token.
type KeyOrUser struct {
	keys *apikeys.Service
	user Authenticator
}

// NewKeyOrUser builds the management authenticator.
func NewKeyOrUser(keys *apikeys.Service, user Authenticator) *KeyOrUser {
	return &KeyOrUser{keys: keys, user: user}
}

// Authenticate implements the authenticator.
func (a *KeyOrUser) Authenticate(r *http.Request) (principal.Principal, error) {
	token, ok := bearerToken(r)
	if !ok || !strings.HasPrefix(token, domain.PrefixAPIKeySecret+"_") {
		return a.user.Authenticate(r)
	}
	userID, err := a.keys.Resolve(r.Context(), token)
	if err != nil {
		return principal.Principal{}, err
	}
	// No session: a key belongs to the account, not to a device. Anything
	// that ends a session therefore cannot be reached with one.
	return principal.User(userID), nil
}
