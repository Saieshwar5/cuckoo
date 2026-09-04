// Package api assembles the server's HTTP surface.
//
// The three APIs — the app's, the agent protocol's, and agent owners' — are
// mounted here under separate prefixes with separate authentication. Handlers
// live in sub-packages; this file only decides what is reachable and what
// guards it.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/agentapi"
	"github.com/Saieshwar5/cuckoo/server/internal/api/client"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/api/mgmt"
	"github.com/Saieshwar5/cuckoo/server/internal/api/middleware"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Deps is everything the HTTP layer needs, supplied by main.
//
// Passing dependencies explicitly rather than reaching for globals is what
// makes the whole router constructible in a test in one line.
type Deps struct {
	Logger *slog.Logger

	// UserAuth identifies people; AgentAuth identifies agent backends. They
	// are different credentials producing different principals, and each API
	// below accepts exactly one of them.
	UserAuth  middleware.Authenticator
	AgentAuth middleware.Authenticator

	Users  *users.Service
	Agents *agents.Service
	Health map[string]HealthCheck
}

// NewRouter builds the server's HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	// chi's RealIP is deliberately absent. It rewrites RemoteAddr from
	// X-Forwarded-For / X-Real-IP, which any client can forge, so the moment
	// anything trusts RemoteAddr — rate limiting by address, abuse blocking —
	// a caller could spoof another user's address or evade their own limit.
	// When real client addresses are needed, they must come from a proxy we
	// configure and trust by name, not from a header we accept from anyone.
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(middleware.Logger(d.Logger))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.NotFound("route_not_found", "No such endpoint."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.Invalid("method_not_allowed",
			"That method is not allowed on this endpoint."))
	})

	r.Get("/healthz", healthHandler(d.Health))

	// The app's API. Every route below requires a signed-in person.
	r.Route("/v1/client", func(r chi.Router) {
		r.Use(middleware.RequireUser(d.UserAuth))
		r.Mount("/", client.New(d.Users).Routes())
	})

	// Agent management: owners creating agents and connecting backends.
	// Also a signed-in person; later also an API key producing the same
	// principal, through the same routes.
	r.Route("/v1/mgmt", func(r chi.Router) {
		r.Use(middleware.RequireUser(d.UserAuth))
		r.Mount("/", mgmt.New(d.Agents).Routes())
	})

	// The agent protocol. Only a binding secret gets in.
	r.Route("/v1/agent", func(r chi.Router) {
		r.Use(middleware.RequireAgent(d.AgentAuth))
		r.Mount("/", agentapi.New(d.Agents).Routes())
	})

	return r
}
