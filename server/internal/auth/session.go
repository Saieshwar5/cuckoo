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
	// A nil service here is a wiring mistake in main that would otherwise
	// surface as a panic on the first request rather than at start-up.
	if s == nil {
		panic("auth: NewSession needs a sign-in service")
	}
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

// bearerToken reads an Authorization: Bearer header, or, for a WebSocket
// opened from a browser, the token offered as a subprotocol.
//
// Browsers cannot set headers on a WebSocket. The one header they do let a
// page fill is the subprotocol list, so the web app offers
// "cuckoo, <session token>" and the hub answers "cuckoo". The token never
// appears in a URL, where it would be logged and cached.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		token = strings.TrimSpace(token)
		return token, token != ""
	}
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, domain.PrefixSessionToken+"_") {
			return p, true
		}
	}
	return "", false
}
