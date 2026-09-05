package auth

import (
	"net/http"
	"strings"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
)

// SessionAuthenticator identifies a person by the session token the app
// presents as a bearer header.
type SessionAuthenticator struct {
	signin *signin.Service
}

// NewSession builds the session authenticator.
func NewSession(s *signin.Service) *SessionAuthenticator {
	return &SessionAuthenticator{signin: s}
}

// Authenticate reads the bearer token and resolves it to a person and their
// session.
func (a *SessionAuthenticator) Authenticate(r *http.Request) (principal.Principal, error) {
	token, ok := bearerToken(r)
	if !ok {
		return principal.Principal{}, domain.Unauthorized("missing_credentials",
			"Send Authorization: Bearer <session token>.")
	}
	userID, sessionID, err := a.signin.ResolveToken(r.Context(), token)
	if err != nil {
		return principal.Principal{}, err
	}
	return principal.UserSession(userID, sessionID), nil
}

// DevOrSession accepts the development header when it is present and a
// session token otherwise, so curl and the app both work against a local
// hub. Only ever constructed in development.
type DevOrSession struct {
	dev     *DevAuthenticator
	session *SessionAuthenticator
}

// NewDevOrSession builds the development-only combination.
func NewDevOrSession(dev *DevAuthenticator, session *SessionAuthenticator) *DevOrSession {
	return &DevOrSession{dev: dev, session: session}
}

// Authenticate implements the authenticator.
func (a *DevOrSession) Authenticate(r *http.Request) (principal.Principal, error) {
	if r.Header.Get(DevHeader) != "" {
		return a.dev.Authenticate(r)
	}
	return a.session.Authenticate(r)
}

// bearerToken reads an Authorization: Bearer header.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
