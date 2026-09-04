package middleware

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// Authenticator identifies the caller behind a request.
//
// The interface is the seam that keeps handlers stable while authentication
// changes underneath them: a development header today, signed session tokens
// next, agent binding secrets after that. Each is an implementation; none of
// them is visible to a handler.
type Authenticator interface {
	// Authenticate returns the caller, or a domain error explaining the refusal.
	Authenticate(r *http.Request) (principal.Principal, error)
}

// RequireUser rejects any request that does not carry a valid user identity,
// and places that identity in the request context.
func RequireUser(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := a.Authenticate(r)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}

			if !p.IsUser() {
				httpx.Error(w, r, domain.Forbidden("user_required",
					"This endpoint is for people using the app."))
				return
			}

			next.ServeHTTP(w, r.WithContext(principal.NewContext(r.Context(), p)))
		})
	}
}

// RequireAgent rejects any request that does not carry a valid agent binding
// secret, and places the agent identity in the request context.
func RequireAgent(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := a.Authenticate(r)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}

			if !p.IsAgent() {
				httpx.Error(w, r, domain.Forbidden("agent_required",
					"This endpoint is for agent backends."))
				return
			}

			next.ServeHTTP(w, r.WithContext(principal.NewContext(r.Context(), p)))
		})
	}
}
