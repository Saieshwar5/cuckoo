// Package auth turns credentials into a principal.
//
// Today it holds only the development authenticator. Magic-link email, Google
// OAuth, session tokens and agent binding secrets land here, each as another
// implementation of middleware.Authenticator, without any handler changing.
package auth

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// DevHeader names the user a development request acts as.
const DevHeader = "X-Dev-User"

// DevAuthenticator trusts an identifier supplied in a request header.
//
// It exists so the rest of the server can be built and exercised before real
// authentication is written: an entire chat protocol can be developed against
// curl without an inbox in the loop. It is constructed only when the process is
// started with CUCKOO_ENV=dev, and startup fails outright in production rather
// than falling back to it — a bypass that can be reached by configuration
// accident is a bypass that will be.
type DevAuthenticator struct{}

// NewDev builds the development authenticator.
func NewDev() *DevAuthenticator { return &DevAuthenticator{} }

// Authenticate reads the development header and returns the named user.
//
// The user is not looked up: an identifier that parses but names nobody reaches
// the handler and fails there as a normal not-found, which is the same path a
// deleted account takes.
func (a *DevAuthenticator) Authenticate(r *http.Request) (principal.Principal, error) {
	raw := r.Header.Get(DevHeader)
	if raw == "" {
		return principal.Principal{}, domain.Unauthorized("missing_credentials",
			"Send "+DevHeader+" with a user id while running in development.")
	}

	id, err := domain.ParseID(domain.PrefixUser, raw)
	if err != nil {
		return principal.Principal{}, domain.Unauthorized("invalid_credentials",
			DevHeader+" must be a user id such as usr_01j7k2m3n4p5q6r7s8t9v0w1x2.")
	}

	return principal.User(id), nil
}
